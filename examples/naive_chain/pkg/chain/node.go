// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package chain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	smart "github.com/hyperledger-labs/SmartBFT/pkg/api"
	smartbft "github.com/hyperledger-labs/SmartBFT/pkg/consensus"
	bft "github.com/hyperledger-labs/SmartBFT/pkg/types"
	"github.com/hyperledger-labs/SmartBFT/pkg/wal"
	"github.com/hyperledger-labs/SmartBFT/smartbftprotos"
	pb "github.com/nastya-biran/SmartBFT/examples/naive_chain/pkg/chain/proto"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type (
	Ingress map[int]<-chan proto.Message
	Egress  map[int]chan<- proto.Message
)

type NetworkOptions struct {
	NumNodes     int
	BatchSize    uint64
	BatchTimeout time.Duration
}

type Node struct {
	clock         *time.Ticker
	secondClock   *time.Ticker
	stopChan      chan struct{}
	doneWG        sync.WaitGroup
	prevHash      string
	id            uint64
	deliverChan   chan<- *Block
	consensus     *smartbft.Consensus
	
	// gRPC клиенты для других нод
	clients       map[uint64]pb.ConsensusServiceClient
	// gRPC сервер
	grpcServer    *grpc.Server
	// Адреса нод
	nodeAddresses map[uint64]string
}

// ConsensusServiceServer реализация gRPC сервера
type consensusServer struct {
	pb.UnimplementedConsensusServiceServer
	node *Node
}

func (s *consensusServer) SendConsensusMessage(ctx context.Context, req *pb.ConsensusMessageRequest) (*pb.ConsensusMessageResponse, error) {
	fmt.Printf("Node %d получил консенсус-сообщение от %d типа %T\n", 
		s.node.id, req.FromNode, req.Message.GetContent())
	
	// Обрабатываем сообщение
	s.node.HandleMessage(req.FromNode, req.Message)
	
	return &pb.ConsensusMessageResponse{
		Success: true,
	}, nil
}

func (*Node) RequestID(req []byte) bft.RequestInfo {
	txn := TransactionFromBytes(req)
	return bft.RequestInfo{
		ClientID: txn.ClientID,
		ID:       txn.ID,
	}
}

func (*Node) VerifyProposal(proposal bft.Proposal) ([]bft.RequestInfo, error) {
	header := BlockHeaderFromBytes(proposal.Header)
	fmt.Printf("Verifying proposal with sequence %d\n", header.Sequence)
	
	blockData := BlockDataFromBytes(proposal.Payload)
	requests := make([]bft.RequestInfo, 0)
	
	for _, t := range blockData.Transactions {
		tx := TransactionFromBytes(t)
		fmt.Printf("Verifying transaction in proposal: client %s, ID %s tx%v t%v\n", tx.ClientID, tx.ID, tx, t)
		if tx.ClientID == "" || tx.ID == "" {
			fmt.Errorf("invalid transaction in proposal: missing ClientID or ID")
			return nil, fmt.Errorf("invalid transaction in proposal: missing ClientID or ID")
		}
		reqInfo := bft.RequestInfo{ID: tx.ID, ClientID: tx.ClientID}
		requests = append(requests, reqInfo)
	}
	
	if len(requests) == 0 {
		return nil, fmt.Errorf("empty proposal: no transactions")
	}
	
	fmt.Printf("Proposal verification successful: %d transactions\n", len(requests))
	// time.Sleep(1 * time.Second)
	return requests, nil
}

func (*Node) RequestsFromProposal(proposal bft.Proposal) []bft.RequestInfo {
	blockData := BlockDataFromBytes(proposal.Payload)
	requests := make([]bft.RequestInfo, 0)
	
	for _, t := range blockData.Transactions {
		tx := TransactionFromBytes(t)
		fmt.Printf("Extracting transaction from proposal: client %s, ID %s\n", tx.ClientID, tx.ID)
		reqInfo := bft.RequestInfo{ID: tx.ID, ClientID: tx.ClientID}
		requests = append(requests, reqInfo)
	}
	
	fmt.Printf("Extracted %d transactions from proposal\n", len(requests))
	return requests
}

func (*Node) VerifyRequest(val []byte) (bft.RequestInfo, error) {
	txn := TransactionFromBytes(val)
	fmt.Printf("Verifying request from client %s with ID %s\n", txn.ClientID, txn.ID)
	if txn.ClientID == "" || txn.ID == "" {
		return bft.RequestInfo{}, fmt.Errorf("invalid transaction: missing ClientID or ID")
	}
	return bft.RequestInfo{
		ClientID: txn.ClientID,
		ID:       txn.ID,
	}, nil
}

func (*Node) VerifyConsenterSig(sig bft.Signature, proposal bft.Proposal) ([]byte, error) {
	fmt.Printf("Verifying consenter signature from node %d\n", sig.ID)
	if sig.ID > 0 {
		header := BlockHeaderFromBytes(proposal.Header)
		fmt.Printf("Signature verified for proposal with sequence %d\n", header.Sequence)
		// Create a PreparesFrom message and encode it as protobuf
        prpf := &smartbftprotos.PreparesFrom{
            Ids: []uint64{sig.ID}, // Wrap the ID in a repeated field
        }
        aux, err := proto.Marshal(prpf) // Serialize to protobuf binary
        if err != nil {
            return nil, fmt.Errorf("failed to marshal PreparesFrom: %w", err)
        }
        return aux, nil
	}
	// time.Sleep(1 * time.Second)
	return nil, fmt.Errorf("invalid signature: ID must be positive")
}

func (*Node) VerifySignature(signature bft.Signature) error {
	fmt.Printf("Verifying signature from node %d\n", signature.ID)
	if signature.ID > 0 {
		return nil
	}
	// time.Sleep(1 * time.Second)
	return fmt.Errorf("invalid signature: ID must be positive")
}

func (*Node) VerificationSequence() uint64 {
	return 0
}

func (*Node) Sign(msg []byte) []byte {
	//time.Sleep(1 * time.Second)
	return msg
}

func (n *Node) SignProposal(proposal bft.Proposal, _ []byte) *bft.Signature {
	header := BlockHeaderFromBytes(proposal.Header)
	fmt.Printf("Node %d signing proposal with sequence %d\n", n.id, header.Sequence)
	//time.Sleep(1 * time.Second)
	
	return &bft.Signature{
		ID:    n.id,
		Value: []byte(fmt.Sprintf("sig-from-%d-for-%d", n.id, header.Sequence)),
	}
}

func (n *Node) AssembleProposal(metadata []byte, requests [][]byte) bft.Proposal {
	fmt.Printf("Node %d assembling proposal with %d requests\n", n.id, len(requests))
	
	blockData := BlockData{Transactions: requests}
	blockDataBytes := blockData.ToBytes()
	
	md := &smartbftprotos.ViewMetadata{}
	if err := proto.Unmarshal(metadata, md); err != nil {
		panic(fmt.Sprintf("Unable to unmarshal metadata, error: %v", err))
	}
	
	header := BlockHeader{
		PrevHash: n.prevHash,
		DataHash: computeDigest(blockDataBytes),
		Sequence: int64(md.LatestSequence),
		ViewId:  int64(md.ViewId),
	}
	headerBytes := header.ToBytes()
	
	fmt.Printf("Node %d created proposal: seq=%d, prevHash=%s, dataHash=%s\n",
		n.id, header.Sequence, header.PrevHash, header.DataHash)
	
	return bft.Proposal{
		Header:   headerBytes,
		Payload:  blockDataBytes,
		Metadata: metadata,
	}
}

func (n *Node) SendConsensus(targetID uint64, message *smartbftprotos.Message) {
	/*fmt.Printf("Node %d пытается отправить сообщение узлу %d типа %T\n", 
		n.id, targetID, message.GetContent())*/

	client, ok := n.clients[targetID]
	if !ok {
		fmt.Printf("Node %d: клиент для узла %d не найден\n", n.id, targetID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.ConsensusMessageRequest{
		FromNode: n.id,
		ToNode:   targetID,
		Message:  message,
	}

	//time.Sleep(1 * time.Second)
	_, err := client.SendConsensusMessage(ctx, req)
	if err != nil {
		fmt.Printf("Node %d: ошибка отправки сообщения узлу %d: %v\n", n.id, targetID, err)
		return
	}

	//fmt.Printf("Node %d успешно отправил сообщение узлу %d\n", n.id, targetID)
}

func (n *Node) SendTransaction(targetID uint64, request []byte) {
	client, ok := n.clients[targetID]
	if !ok {
		fmt.Printf("Node %d: клиент для узла %d не найден\n", n.id, targetID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx := TransactionFromBytes(request)

	req := &pb.TransactionRequest{
		FromNode: n.id,
		ToNode:   targetID,
		Tx : &pb.Transaction{ClientId: tx.ClientID, Id: tx.ID},
	}

	//time.Sleep(1 * time.Second)
	_, err := client.SendTransaction(ctx, req)
	if err != nil {
		fmt.Printf("Node %d: ошибка отправки транзакции узлу %d: %v\n", n.id, targetID, err)
		return
	}

	fmt.Printf("Node %d успешно отправил транзакцию узлу %d %s\n", n.id, targetID, n.RequestID(request))
}

func (n *Node) MembershipChange() bool {
	return false
}

func (n *Node) Deliver(proposal bft.Proposal, signature []bft.Signature) bft.Reconfig {
	blockData := BlockDataFromBytes(proposal.Payload)
	txns := make([]Transaction, 0, len(blockData.Transactions))
	for _, rawTxn := range blockData.Transactions {
		txn := TransactionFromBytes(rawTxn)
		txns = append(txns, Transaction{
			ClientID: txn.ClientID,
			ID:       txn.ID,
		})
	}
	header := BlockHeaderFromBytes(proposal.Header)

	select {
	case <-n.stopChan:
		return bft.Reconfig{InLatestDecision: false}
	case n.deliverChan <- &Block{
		Sequence:     uint64(header.Sequence),
		PrevHash:     header.PrevHash,
		Transactions: txns,
	}:
	}

	return bft.Reconfig{InLatestDecision: false}
}

func NewNode(id uint64, nodeAddresses map[uint64]string, deliverChan chan<- *Block, logger smart.Logger, walmet *wal.Metrics, bftmet *smart.Metrics, opts NetworkOptions, testDir string) *Node {
	nodeDir := filepath.Join(testDir, fmt.Sprintf("node%d", id))

	writeAheadLog, err := wal.Create(logger, nodeDir, &wal.Options{Metrics: walmet.With("label1", "val1")})
	if err != nil {
		logger.Panicf("Cannot create WAL at %s", nodeDir)
	}

	node := &Node{
		clock:         time.NewTicker(time.Second),
		secondClock:   time.NewTicker(time.Second),
		id:            id,
		deliverChan:   deliverChan,
		stopChan:      make(chan struct{}),
		nodeAddresses: nodeAddresses,
		clients:       make(map[uint64]pb.ConsensusServiceClient),
	}

	config := bft.DefaultConfig
	config.SelfID = id
	config.RequestBatchMaxInterval = opts.BatchTimeout
	config.RequestBatchMaxCount = opts.BatchSize

	node.consensus = &smartbft.Consensus{
		Config:             config,
		ViewChangerTicker:  node.secondClock.C,
		Scheduler:          node.clock.C,
		Logger:             logger,
		Metrics:            bftmet,
		Comm:               node,
		Signer:             node,
		MembershipNotifier: node,
		Verifier:           node,
		Application:        node,
		Assembler:          node,
		RequestInspector:   node,
		Synchronizer:       node,
		WAL:                writeAheadLog,
		Metadata: &smartbftprotos.ViewMetadata{
			LatestSequence: 0,
			ViewId:         0,
		},
	}
	if err = node.consensus.Start(); err != nil {
		panic("error on consensus start")
	}
	node.Start()
	return node
}

// InitializeClients инициализирует gRPC клиенты для связи с другими нодами
func (n *Node) InitializeClients() error {
	// Создаем клиенты для других нод
	for targetID, addr := range n.nodeAddresses {
		if targetID == n.id {
			continue
		}
		
		// Пытаемся подключиться с повторами
		var conn *grpc.ClientConn
		var err error
		for i := 0; i < 5; i++ { // 5 попыток
			conn, err = grpc.Dial(addr, grpc.WithInsecure(), grpc.WithBlock(), 
				grpc.WithTimeout(5*time.Second))
			if err == nil {
				break
			}
			time.Sleep(2 * time.Second) // Ждем 2 секунды между попытками
		}
		if err != nil {
			return fmt.Errorf("failed to connect to node %d after retries: %v", targetID, err)
		}
		n.clients[targetID] = pb.NewConsensusServiceClient(conn)
	}
	return nil
}

func (n *Node) Start() {
	fmt.Printf("Node %d starting\n", n.id)
}

func (n *Node) Stop() {
	select {
	case <-n.stopChan:
		break
	default:
		close(n.stopChan)
	}
	n.clock.Stop()
	n.secondClock.Stop()
	
	// Останавливаем gRPC сервер
	if n.grpcServer != nil {
		n.grpcServer.GracefulStop()
	}
	
	// Закрываем все клиентские соединения
	for _, client := range n.clients {
		if conn, ok := client.(interface{ Close() error }); ok {
			conn.Close()
		}
	}
	
	n.consensus.Stop()
}

func (n *Node) Nodes() []uint64 {
	nodes := make([]uint64, 0, len(n.nodeAddresses))
	for id := range n.nodeAddresses {
		nodes = append(nodes, id)
	}
	return nodes
}

func computeDigest(rawBytes []byte) string {
	h := sha256.New()
	h.Write(rawBytes)
	digest := h.Sum(nil)
	return hex.EncodeToString(digest)
}

func (n *Node) HandleMessage(fromNode uint64, msg *smartbftprotos.Message) error {
	/*fmt.Printf("Node %d обрабатывает сообщение от %d типа %T\n", 
		n.id, fromNode, msg.GetContent())*/
	
	// Передаем сообщение в консенсус
	n.consensus.HandleMessage(fromNode, msg)
	return nil
}

func (n *Node) StartViewChange(view uint64) {
	fmt.Printf("Node %d initiating view change to view %d\n", n.id, view)
	
	viewChange := &smartbftprotos.Message{
		Content: &smartbftprotos.Message_ViewChange{
			ViewChange: &smartbftprotos.ViewChange{
				NextView: view,
			},
		},
	}
	
	// Отправляем ViewChange всем узлам
	for nodeID := range n.nodeAddresses {
		if nodeID == n.id {
			continue
		}
		n.SendConsensus(nodeID, viewChange)
	}
}

func (n *Node) Sync() bft.SyncResponse {
	fmt.Printf("Node %d: Sync called\n", n.id)
	
	// Получаем последний блок
	header := BlockHeader{
		Sequence: 1, // Начинаем с 1
		PrevHash: n.prevHash,
		ViewId: 0,
	}
	
	metadata := &smartbftprotos.ViewMetadata{
		ViewId:         0,
		LatestSequence: 1,
	}
	metadataBytes, err := proto.Marshal(metadata)
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal metadata: %v", err))
	}
	
	return bft.SyncResponse{
		Latest: bft.Decision{
			Proposal: bft.Proposal{
				Header:   header.ToBytes(),
				Metadata: metadataBytes,
			},
			Signatures: nil, // Для первого блока подписи не нужны
		},
		Reconfig: bft.ReconfigSync{InReplicatedDecisions: false},
	}
}

// Исправляем метод AuxiliaryData для интерфейса api.Verifier
func (*Node) AuxiliaryData(bytes []byte) []byte {
	return nil // Возвращаем nil, так как у нас нет дополнительных данных
}

func (n *Node) BroadcastSpamMessage(){
	header := BlockHeaderFromBytes(n.consensus.LastProposal.Header)
	fmt.Printf("Spam %d %d \n", header.Sequence, header.ViewId)
	msg :=  &smartbftprotos.Message{
		Content: &smartbftprotos.Message_PrePrepare{
			PrePrepare : &smartbftprotos.PrePrepare{
				View: n.consensus.Controller.cu
				Seq:  uint64(header.Sequence),
				Proposal: &smartbftprotos.Proposal{
					Header:  []byte{0},
					Payload: []byte{1},
					Metadata: []byte{0},
					VerificationSequence: 0,
				},
			}, 
		},
	}
	
	for _, node := range n.Nodes() {
	// Do not send to yourself
		if n.id == node {
			continue
		}
		n.SendConsensus(node, msg)
	}
}

// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package chain

import (
	"bytes"
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
	clock       *time.Ticker
	secondClock *time.Ticker
	stopChan    chan struct{}
	doneWG      sync.WaitGroup
	prevHash    string
	id          uint64
	deliverChan chan<- *Block
	consensus   *smartbft.Consensus

	clients       map[uint64]pb.ConsensusServiceClient
	grpcServer    *grpc.Server
	nodeAddresses map[uint64]string

	delivered_proposals  map[int64]bft.Proposal
	maxDeliveredSequence int64

	isByzantine bool
}

type consensusServer struct {
	pb.UnimplementedConsensusServiceServer
	node *Node
}

var MyDefaultConfig = bft.Configuration{
	RequestBatchMaxCount:          100,
	RequestBatchMaxBytes:          10 * 1024 * 1024,
	RequestBatchMaxInterval:       50 * time.Millisecond,
	IncomingMessageBufferSize:     10000,
	RequestPoolSize:               400,
	RequestForwardTimeout:         2 * time.Second,
	RequestComplainTimeout:        20 * time.Second,
	RequestAutoRemoveTimeout:      3 * time.Minute,
	ViewChangeResendInterval:      5 * time.Second,
	ViewChangeTimeout:             20 * time.Second,
	LeaderHeartbeatTimeout:        time.Minute,
	LeaderHeartbeatCount:          10,
	NumOfTicksBehindBeforeSyncing: 10,
	CollectTimeout:                time.Second,
	SyncOnStart:                   false,
	SpeedUpViewChange:             false,
	LeaderRotation:                true,
	DecisionsPerLeader:            3,
	RequestMaxBytes:               10 * 1024,
	RequestPoolSubmitTimeout:      5 * time.Second,
}

const NetworkLatency = 50
const CryptoLatency = 3
const VerifyProposalLatency = 10

func Delay(count int) int {
	start := time.Now()
	i := 0
	for time.Since(start).Milliseconds() < int64(count) {
		i++
	}
	return i
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

	Delay(VerifyProposalLatency)

	blockData := BlockDataFromBytes(proposal.Payload)
	requests := make([]bft.RequestInfo, 0)

	for _, t := range blockData.Transactions {
		tx := TransactionFromBytes(t)
		fmt.Printf("Verifying transaction in proposal: client %s, ID %s tx%v t%v\n", tx.ClientID, tx.ID, tx, t)
		if tx.ClientID == "" || tx.ID == "" {
			fmt.Errorf("invalid transaction in proposal: missing ClientID or ID")
			return nil, fmt.Errorf("invalid transaction in proposal: missing ClientID or ID")
		}
		if tx.ClientID == "faulty" {
			return nil, fmt.Errorf("invalid transaction")
		}
		reqInfo := bft.RequestInfo{ID: tx.ID, ClientID: tx.ClientID}
		requests = append(requests, reqInfo)
	}

	if len(requests) == 0 {
		return nil, fmt.Errorf("empty proposal: no transactions")
	}
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
	Delay(VerifyProposalLatency)
	if txn.ClientID == "" || txn.ID == "" {
		return bft.RequestInfo{}, fmt.Errorf("invalid transaction: missing ClientID or ID")
	}
	if txn.ClientID == "faulty" {
		return bft.RequestInfo{}, fmt.Errorf("invalid transaction")
	}
	return bft.RequestInfo{
		ClientID: txn.ClientID,
		ID:       txn.ID,
	}, nil
}

func (*Node) VerifyConsenterSig(sig bft.Signature, proposal bft.Proposal) ([]byte, error) {
	fmt.Printf("Verifying consenter signature from node %d\n", sig.ID)
	Delay(CryptoLatency)
	if sig.ID > 0 {
		header := BlockHeaderFromBytes(proposal.Header)
		fmt.Printf("Signature verified for proposal with sequence %d\n", header.Sequence)
		prpf := &smartbftprotos.PreparesFrom{
			Ids: []uint64{sig.ID},
		}
		aux, err := proto.Marshal(prpf)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal PreparesFrom: %w", err)
		}
		return aux, nil
	}
	return nil, fmt.Errorf("invalid signature: ID must be positive")
}

func (*Node) VerifySignature(signature bft.Signature) error {
	fmt.Printf("Verifying signature from node %d\n", signature.ID)
	Delay(CryptoLatency)
	if signature.ID > 0 {
		return nil
	}
	return fmt.Errorf("invalid signature: ID must be positive")
}

func (*Node) VerificationSequence() uint64 {
	return 0
}

func (n *Node) Sign(msg []byte) []byte {
	fmt.Printf("Node %d signing message\n", n.id)
	Delay(CryptoLatency)
	return msg
}

func (n *Node) SignProposal(proposal bft.Proposal, _ []byte) *bft.Signature {
	header := BlockHeaderFromBytes(proposal.Header)
	fmt.Printf("Node %d signing proposal with sequence %d\n", n.id, header.Sequence)
	Delay(CryptoLatency)

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
		ViewId:   int64(md.ViewId),
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

	go func() {
		client, ok := n.clients[targetID]
		if !ok {
			fmt.Printf("Node %d: клиент для узла %d не найден\n", n.id, targetID)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req := &pb.ConsensusMessageRequest{
			FromNode: n.id,
			ToNode:   targetID,
			Message:  message,
		}
		_, err := client.SendConsensusMessage(ctx, req)
		if err != nil {
			fmt.Printf("Node %d: ошибка отправки сообщения узлу %d: %v\n", n.id, targetID, err)
			return
		}
	}()
}

func (n *Node) SendTransaction(targetID uint64, request []byte) {
	client, ok := n.clients[targetID]
	if !ok {
		fmt.Printf("Node %d: клиент для узла %d не найден\n", n.id, targetID)
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		tx := TransactionFromBytes(request)

		req := &pb.TransactionRequest{
			FromNode: n.id,
			ToNode:   targetID,
			Tx:       &pb.Transaction{ClientId: tx.ClientID, Id: tx.ID},
		}
		_, err := client.SendTransaction(ctx, req)
		if err != nil {
			fmt.Printf("Node %d: ошибка отправки транзакции узлу %d: %v\n", n.id, targetID, err)
			return
		}
	}()
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
	fmt.Printf("before delivered_proposal\n")
	n.delivered_proposals[header.Sequence] = proposal
	if n.maxDeliveredSequence >= header.Sequence {
		panic("Delivering old proposal")
	}
	n.maxDeliveredSequence = header.Sequence
	fmt.Printf("after\n")

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

func NewNode(id uint64, nodeAddresses map[uint64]string, deliverChan chan<- *Block, logger smart.Logger, walmet *wal.Metrics, bftmet *smart.Metrics, opts NetworkOptions, testDir string, isByzantine bool) *Node {
	nodeDir := filepath.Join(testDir, fmt.Sprintf("node%d", id))

	writeAheadLog, err := wal.Create(logger, nodeDir, &wal.Options{Metrics: walmet.With("label1", "val1")})
	if err != nil {
		logger.Panicf("Cannot create WAL at %s", nodeDir)
	}

	node := &Node{
		clock:                time.NewTicker(time.Second),
		secondClock:          time.NewTicker(time.Second),
		id:                   id,
		deliverChan:          deliverChan,
		stopChan:             make(chan struct{}),
		nodeAddresses:        nodeAddresses,
		clients:              make(map[uint64]pb.ConsensusServiceClient),
		delivered_proposals:  make(map[int64]bft.Proposal),
		maxDeliveredSequence: 0,
		isByzantine:          isByzantine,
	}

	if !isByzantine {
		config := MyDefaultConfig
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
			panic(fmt.Sprintf("error on consensus start %s", err))
		}
	}

	node.Start()
	return node
}

func (n *Node) InitializeClients() error {
	for targetID, addr := range n.nodeAddresses {
		if targetID == n.id {
			continue
		}

		var conn *grpc.ClientConn
		var err error
		for i := 0; i < 5; i++ {
			conn, err = grpc.Dial(addr, grpc.WithInsecure(), grpc.WithBlock(),
				grpc.WithTimeout(5*time.Second))
			if err == nil {
				break
			}
			time.Sleep(2 * time.Second)
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

	if n.grpcServer != nil {
		n.grpcServer.GracefulStop()
	}

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
	n.consensus.HandleMessage(fromNode, msg)
	return nil
}

func (n *Node) HandleRequest(fromNode uint64, req []byte) error {
	n.consensus.HandleRequest(fromNode, req)
	return nil
}

func (n *Node) Sync() bft.SyncResponse {
	fmt.Printf("Node %d: Sync called\n", n.id)

	curSeq := n.maxDeliveredSequence

	for {
		fmt.Printf("Trying to get seq %d in sync\n", curSeq)
		var lock sync.Mutex
		var wg sync.WaitGroup
		var proposalForSeq *bft.Proposal = nil

		wg.Add(len(n.nodeAddresses) - 1)
		for nodeID := range n.nodeAddresses {
			if nodeID == n.id {
				continue
			}

			client, ok := n.clients[nodeID]
			if !ok {
				panic(fmt.Sprintf("Node %d: клиент для узла %d не найден\n", n.id, nodeID))
			}

			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				defer wg.Done()

				syncMsg := &pb.SyncRequest{
					Sequence: uint64(curSeq),
					FromNode: n.id,
				}

				proposal, err := client.Sync(ctx, syncMsg)
				if err != nil {
					fmt.Printf("Node %d: ошибка отправки транзакции узлу %d: %v\n", n.id, nodeID, err)
					return
				}
				fmt.Printf("Got sync proposal for seq %d, has_proposal %t from node %d\n", proposal.Sequence, proposal.HasProposal, nodeID)
				if !proposal.HasProposal {
					return
				}
				lock.Lock()
				proposal_tmp := &bft.Proposal{
					Header:   proposal.Proposal.Header,
					Payload:  proposal.Proposal.Payload,
					Metadata: proposal.Proposal.Metadata,
				}
				if proposalForSeq != nil && (!bytes.Equal(proposal_tmp.Header, proposalForSeq.Header) || !bytes.Equal(proposal_tmp.Metadata, proposalForSeq.Metadata) || !bytes.Equal(proposal_tmp.Payload, proposalForSeq.Payload)) {
					panic("Delivered proposals dont match")
				}
				proposalForSeq = proposal_tmp
				lock.Unlock()
			}()
		}
		wg.Wait()
		lock.Lock()
		if proposalForSeq == nil {
			lock.Unlock()
			break
		}
		n.delivered_proposals[int64(curSeq)] = *proposalForSeq
		lock.Unlock()
		curSeq += 1
	}

	fmt.Printf("Returning from sync seq %d\n", BlockHeaderFromBytes(n.delivered_proposals[int64(curSeq)-1].Header).Sequence)

	return bft.SyncResponse{
		Latest: bft.Decision{
			Proposal:   n.delivered_proposals[int64(curSeq)-1],
			Signatures: nil,
		},
		Reconfig: bft.ReconfigSync{InReplicatedDecisions: false},
	}

}

func (*Node) AuxiliaryData(bytes []byte) []byte {
	return nil
}

func (n *Node) BroadcastSpamMessage(count uint64, round uint64) {

	for i := uint64(1); i <= count; i++ {
		for nodeID := range n.nodeAddresses {
			if nodeID == n.id {
				continue
			}
			msg := &pb.TransactionRequest{
				FromNode: n.id,
				ToNode:   nodeID,
				Tx: &pb.Transaction{
					ClientId: "faulty",
					Id:       fmt.Sprintf("%d-%d", i, round),
				},
			}
			client, ok := n.clients[nodeID]
			if !ok {
				fmt.Printf("Node %d: клиент для узла %d не найден\n", n.id, nodeID)
				return
			}

			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				_, err := client.SendTransaction(ctx, msg)
				if err != nil {
					return
				}
			}()
		}
	}

}

func (n *Node) GetDeliveredProposal(seq int64) (*bft.Proposal, error) {
	if value, exists := n.delivered_proposals[seq]; exists {
		return &value, nil
	}

	return nil, fmt.Errorf("Proposal not found")
}

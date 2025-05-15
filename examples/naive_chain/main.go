package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	smart "github.com/hyperledger-labs/SmartBFT/pkg/api"
	"github.com/hyperledger-labs/SmartBFT/pkg/metrics/disabled"
	"github.com/hyperledger-labs/SmartBFT/pkg/wal"
	"github.com/nastya-biran/SmartBFT/examples/naive_chain/pkg/chain"
	pb "github.com/nastya-biran/SmartBFT/examples/naive_chain/pkg/chain/proto"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type transactionServer struct {
	pb.UnimplementedTransactionServiceServer
	chain *chain.Chain
}

type consensusServer struct {
	pb.UnimplementedConsensusServiceServer
	chain *chain.Chain
}

const NetworkLatency = 50
const CryptoLatency = 3
const VerifyProposalLatency = 10

func (s *consensusServer) SendConsensusMessage(ctx context.Context, req *pb.ConsensusMessageRequest) (*pb.ConsensusMessageResponse, error) {
	time.Sleep(time.Duration(CryptoLatency+NetworkLatency+rand.Intn(21)-10) * time.Millisecond)

	if !s.chain.IsByzantine() {
		err := s.chain.HandleMessage(req.FromNode, req.Message)
		if err != nil {
			return &pb.ConsensusMessageResponse{
				Success: false,
				Error:   err.Error(),
			}, nil
		}
	}

	return &pb.ConsensusMessageResponse{
		Success: true,
	}, nil
}

func (s *consensusServer) SendTransaction(ctx context.Context, req *pb.TransactionRequest) (*pb.TransactionResponse, error) {
	time.Sleep(time.Duration(CryptoLatency+NetworkLatency+rand.Intn(21)-10) * time.Millisecond)

	tx := chain.Transaction{
		ClientID: req.Tx.ClientId,
		ID:       req.Tx.Id,
	}

	if !s.chain.IsByzantine() {
		err := s.chain.HandleRequest(req.FromNode, tx.ToBytes())
		if err != nil {
			return &pb.TransactionResponse{
				Success: false,
				Error:   err.Error(),
			}, nil
		}
	}

	return &pb.TransactionResponse{
		Success: true,
	}, nil
}

func (s *consensusServer) Sync(ctx context.Context, req *pb.SyncRequest) (*pb.SyncResponse, error) {
	time.Sleep(time.Duration(CryptoLatency + +rand.Intn(21) - 10) * time.Millisecond)
	fmt.Printf("Node %d called sync\n", req.FromNode)

	if s.chain.IsByzantine() || req.Sequence > s.chain.GetCurrentSequence() {
		return &pb.SyncResponse{
			Sequence:    req.Sequence,
			HasProposal: false,
		}, nil
	}

	proposal, error := s.chain.GetDeliveredProposal(int64(req.Sequence))
	if error != nil {
		return &pb.SyncResponse{
			Sequence:    req.Sequence,
			HasProposal: false,
		}, nil
	}

	return &pb.SyncResponse{
		Sequence:    req.Sequence,
		HasProposal: true,
		Proposal:    proposal,
		Signatures:  nil,
	}, nil

}

func (s *transactionServer) SubmitTransaction(ctx context.Context, req *pb.ClientTransactionRequest) (*pb.TransactionResponse, error) {
	if !s.chain.IsByzantine() {
		err := s.chain.Order(chain.Transaction{
			ClientID: req.Tx.ClientId,
			ID:       req.Tx.Id,
		})

		if err != nil {
			return &pb.TransactionResponse{
				Success: false,
				Error:   err.Error(),
			}, nil
		}
	}

	return &pb.TransactionResponse{
		Success: true,
	}, nil
}

func main() {
	rand.Seed(time.Now().UnixNano())
	nodeID, err := strconv.ParseUint(os.Getenv("NODE_ID"), 10, 64)
	if err != nil {
		panic(fmt.Sprintf("Invalid NODE_ID: %v", err))
	}

	totalNodes, err := strconv.Atoi(os.Getenv("TOTAL_NODES"))
	if err != nil {
		panic(fmt.Sprintf("Invalid TOTAL_NODES: %v", err))
	}

	nodeAddressesStr := os.Getenv("NODE_ADDRESSES")
	if nodeAddressesStr == "" {
		panic("NODE_ADDRESSES environment variable is not set")
	}

	nodeAddresses := make(map[uint64]string)
	addresses := strings.Split(nodeAddressesStr, ",")
	if len(addresses) != totalNodes {
		panic(fmt.Sprintf("Expected %d node addresses, got %d", totalNodes, len(addresses)))
	}

	for i, addr := range addresses {
		nodeAddresses[uint64(i+1)] = addr
	}

	nodeAddress := os.Getenv("NODE_ADDRESS")
	if nodeAddress == "" {
		panic("NODE_ADDRESS environment variable is not set")
	}

	logConfig := zap.NewDevelopmentConfig()
	logConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	logger, _ := logConfig.Build()
	sugar := logger.Sugar()

	met := &disabled.Provider{}
	walMet := wal.NewMetrics(met, "node")
	bftMet := smart.NewMetrics(met, "node")

	dataDir := "/app/data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create data directory: %v", err))
	}

	opts := chain.NetworkOptions{
		NumNodes:     totalNodes,
		BatchSize:    1,
		BatchTimeout: 10 * time.Second,
	}

	is_byzantine, err := strconv.ParseBool(os.Getenv("IS_BYZANTINE"))
	if err != nil {
		panic(fmt.Sprintf("Invalid IS_BYZANTINE: %v", err))
	}
	c := chain.NewChain(
		nodeID,
		nodeAddresses,
		sugar,
		walMet,
		bftMet,
		opts,
		dataDir,
		is_byzantine,
	)

	txServer := grpc.NewServer()
	pb.RegisterTransactionServiceServer(txServer, &transactionServer{chain: c})
	reflection.Register(txServer)

	txLis, err := net.Listen("tcp", ":7051")
	if err != nil {
		sugar.Fatalf("Failed to listen on transaction port: %v", err)
	}

	go func() {
		if err := txServer.Serve(txLis); err != nil {
			sugar.Fatalf("Failed to start transaction gRPC server: %v", err)
		}
	}()

	consensusSrv := grpc.NewServer()
	pb.RegisterConsensusServiceServer(consensusSrv, &consensusServer{chain: c})
	reflection.Register(consensusSrv)

	consensusLis, err := net.Listen("tcp", ":7050")
	if err != nil {
		sugar.Fatalf("Failed to listen on consensus port: %v", err)
	}

	go func() {
		if err := consensusSrv.Serve(consensusLis); err != nil {
			sugar.Fatalf("Failed to start consensus gRPC server: %v", err)
		}
	}()

	sugar.Infof("Node %d started successfully", nodeID)

	time.Sleep(5 * time.Second)
	if err := c.InitializeClients(); err != nil {
		sugar.Fatalf("Failed to initialize clients: %v", err)
	}
	sugar.Info("Successfully connected to other nodes")

	if is_byzantine {
		round := uint64(1)
		period, err := strconv.ParseUint(os.Getenv("SPAM_MESSAGE_PERIOD"), 10, 64)
		if err != nil {
			panic(fmt.Sprintf("Invalid SPAM_MESSAGE_PERIOD: %v", err))
		}

		count, err := strconv.ParseUint(os.Getenv("SPAM_MESSAGE_COUNT"), 10, 64)
		if err != nil {
			panic(fmt.Sprintf("Invalid SPAM_MESSAGE_COUNT: %v", err))
		}

		ticker := time.NewTicker(time.Duration(period) * time.Millisecond)

		for {
			select {
			case <-ticker.C:
				round++
				c.BroadcastSpamMessage(count, round)
			}
		}
	} else {
		for {
			block := <-c.Listen()
			sugar.Infof("Node %d received block: %+v", nodeID, block)
		}
	}

}

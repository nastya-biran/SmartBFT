package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	
	"go.uber.org/zap"
	"github.com/hyperledger-labs/SmartBFT/pkg/metrics/disabled"
	"github.com/hyperledger-labs/SmartBFT/pkg/wal"
	smart "github.com/hyperledger-labs/SmartBFT/pkg/api"
	"github.com/nastya-biran/SmartBFT/examples/naive_chain/pkg/chain"
	pb "github.com/nastya-biran/SmartBFT/examples/naive_chain/pkg/chain/proto"
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

func (s *consensusServer) SendConsensusMessage(ctx context.Context, req *pb.ConsensusMessageRequest) (*pb.ConsensusMessageResponse, error) {
	err := s.chain.HandleMessage(req.FromNode, req.Message)
	if err != nil {
		return &pb.ConsensusMessageResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	return &pb.ConsensusMessageResponse{
		Success: true,
	}, nil
}

func (s *consensusServer) SendTransaction(ctx context.Context, req *pb.TransactionRequest) (*pb.TransactionResponse, error) {
	// Пока просто пересылаем транзакцию в цепочку
	fmt.Printf("Send transaction in main %d %s %s\n", req.FromNode, req.Tx.Id, req.Tx.ClientId)
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

	return &pb.TransactionResponse{
		Success: true,
	}, nil
}

func (s *transactionServer) SubmitTransaction(ctx context.Context, req *pb.ClientTransactionRequest) (*pb.TransactionResponse, error) {
	// Пока просто пересылаем транзакцию в цепочку
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

	return &pb.TransactionResponse{
		Success: true,
	}, nil
}

func main() {
	// Получаем ID ноды и общее количество нод из переменных окружения
	nodeID, err := strconv.ParseUint(os.Getenv("NODE_ID"), 10, 64)
	if err != nil {
		panic(fmt.Sprintf("Invalid NODE_ID: %v", err))
	}

	totalNodes, err := strconv.Atoi(os.Getenv("TOTAL_NODES"))
	if err != nil {
		panic(fmt.Sprintf("Invalid TOTAL_NODES: %v", err))
	}

	// Получаем адреса нод
	nodeAddressesStr := os.Getenv("NODE_ADDRESSES")
	if nodeAddressesStr == "" {
		panic("NODE_ADDRESSES environment variable is not set")
	}

	// Парсим адреса нод
	nodeAddresses := make(map[uint64]string)
	addresses := strings.Split(nodeAddressesStr, ",")
	if len(addresses) != totalNodes {
		panic(fmt.Sprintf("Expected %d node addresses, got %d", totalNodes, len(addresses)))
	}

	for i, addr := range addresses {
		nodeAddresses[uint64(i+1)] = addr
	}

	// Получаем адрес ноды
	nodeAddress := os.Getenv("NODE_ADDRESS")
	if nodeAddress == "" {
		panic("NODE_ADDRESS environment variable is not set")
	}

	// Настраиваем логгер
	logConfig := zap.NewDevelopmentConfig()
	logConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	logger, _ := logConfig.Build()
	sugar := logger.Sugar()

	// Создаем метрики
	met := &disabled.Provider{}
	walMet := wal.NewMetrics(met, "node")
	bftMet := smart.NewMetrics(met, "node")

	// Создаем директорию для WAL
	dataDir := "/app/data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create data directory: %v", err))
	}

	// Настраиваем опции сети
	opts := chain.NetworkOptions{
		NumNodes:     totalNodes,
		BatchSize:    1,
		BatchTimeout: 10 * time.Second,
	}

	// Создаем и запускаем цепочку
	c := chain.NewChain(
		nodeID,
		nodeAddresses,
		sugar,
		walMet,
		bftMet,
		opts,
		dataDir,
	)

	// Создаем gRPC сервер для транзакций
	txServer := grpc.NewServer()
	pb.RegisterTransactionServiceServer(txServer, &transactionServer{chain: c})
	reflection.Register(txServer)

	// Запускаем gRPC сервер для транзакций на порту 7051
	txLis, err := net.Listen("tcp", ":7051")
	if err != nil {
		sugar.Fatalf("Failed to listen on transaction port: %v", err)
	}

	go func() {
		if err := txServer.Serve(txLis); err != nil {
			sugar.Fatalf("Failed to start transaction gRPC server: %v", err)
		}
	}()

	// Создаем gRPC сервер для консенсуса
	consensusSrv := grpc.NewServer()
	pb.RegisterConsensusServiceServer(consensusSrv, &consensusServer{chain: c})
	reflection.Register(consensusSrv)

	// Запускаем gRPC сервер для консенсуса на порту 7050
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

	// Инициализируем клиенты для связи с другими нодами
	// Даем время на запуск всех серверов
	time.Sleep(5 * time.Second)
	if err := c.InitializeClients(); err != nil {
		sugar.Fatalf("Failed to initialize clients: %v", err)
	}
	sugar.Info("Successfully connected to other nodes")

	is_byzantine, err := strconv.ParseBool(os.Getenv("IS_BYZANTINE"))
	if err != nil {
		panic(fmt.Sprintf("Invalid IS_BYZANTINE: %v", err))
	}

	if is_byzantine {
		period, err := strconv.ParseUint(os.Getenv("SPAM_MESSAGE_PERIOD"), 10, 64)
		if err != nil {
			panic(fmt.Sprintf("Invalid SPAM_MESSAGE_PERIOD: %v", err))
		}

		ticker := time.NewTicker(time.Duration(period) * time.Millisecond)

		for {
			select {
			case block := <-c.Listen():
				sugar.Infof("Node %d received block: %+v", nodeID, block)
			case <-ticker.C:
				sugar.Infof("Ticker")
				c.BroadcastSpamMessage()
			}
		}
	} else {
			// Слушаем блоки
		for {
			block := <-c.Listen()
			sugar.Infof("Node %d received block: %+v", nodeID, block)
		}
	}

	
} 
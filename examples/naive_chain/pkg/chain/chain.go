package chain

import (
	"fmt"
	smart "github.com/hyperledger-labs/SmartBFT/pkg/api"
	"github.com/hyperledger-labs/SmartBFT/pkg/wal"
	"github.com/hyperledger-labs/SmartBFT/smartbftprotos"
)

type Chain struct {
	node        *Node
	deliverChan chan *Block
}

func NewChain(id uint64, nodeAddresses map[uint64]string, logger smart.Logger, walmet *wal.Metrics, bftmet *smart.Metrics, opts NetworkOptions, testDir string) *Chain {
	deliverChan := make(chan *Block, 100)
	node := NewNode(id, nodeAddresses, deliverChan, logger, walmet, bftmet, opts, testDir)
	return &Chain{
		node:        node,
		deliverChan: deliverChan,
	}
}

func (c *Chain) Order(tx Transaction) error {
	if c.node == nil {
		return fmt.Errorf("node is not initialized")
	}
	c.node.consensus.SubmitRequest(tx.ToBytes())
	return nil
}

func (c *Chain) Listen() chan *Block {
	return c.deliverChan
}

func (c *Chain) HandleMessage(fromNode uint64, msg *smartbftprotos.Message) error {
	if c.node == nil {
		return fmt.Errorf("node is not initialized")
	}
	return c.node.HandleMessage(fromNode, msg)
}

func (c *Chain) InitializeClients() error {
	if c.node == nil {
		return fmt.Errorf("node is not initialized")
	}
	return c.node.InitializeClients()
} 

func (c *Chain) BroadcastSpamMessage() {
	c.node.BroadcastSpamMessage()
}
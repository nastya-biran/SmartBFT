package chain

import (
	"encoding/asn1"
)

type Transaction struct {
	ClientID string
	ID       string
}

type Block struct {
	Sequence     uint64
	PrevHash     string
	Transactions []Transaction
}

type BlockHeader struct {
	Sequence int64
	PrevHash string
	DataHash string
}

type BlockData struct {
	Transactions [][]byte
}

func (t Transaction) ToBytes() []byte {
	rawTxn, err := asn1.Marshal(t)
	if err != nil {
		panic(err)
	}
	return rawTxn
}

func TransactionFromBytes(bytes []byte) *Transaction {
	var txn Transaction
	asn1.Unmarshal(bytes, &txn)
	return &txn
}

func (b BlockHeader) ToBytes() []byte {
	rawHeader, err := asn1.Marshal(b)
	if err != nil {
		panic(err)
	}
	return rawHeader
}

func BlockHeaderFromBytes(bytes []byte) *BlockHeader {
	var header BlockHeader
	asn1.Unmarshal(bytes, &header)
	return &header
}

func (b BlockData) ToBytes() []byte {
	rawBlock, err := asn1.Marshal(b)
	if err != nil {
		panic(err)
	}
	return rawBlock
}

func BlockDataFromBytes(bytes []byte) *BlockData {
	var block BlockData
	asn1.Unmarshal(bytes, &block)
	return &block
} 
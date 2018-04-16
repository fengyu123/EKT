package blockchain

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/EducationEKT/EKT/io/ekt8/MPTPlus"
	"github.com/EducationEKT/EKT/io/ekt8/core/common"
	"github.com/EducationEKT/EKT/io/ekt8/crypto"
	"github.com/EducationEKT/EKT/io/ekt8/db"
	"github.com/EducationEKT/EKT/io/ekt8/i_consensus"
)

var currentBlock *Block = nil

type Block struct {
	Height       int64              `json:"height"`
	Nonce        int64              `json:"nonce"`
	Fee          int64              `json:"fee"`
	TotalFee     int64              `json:"totalFee"`
	PreviousHash []byte             `json:"previousHash"`
	CurrentHash  []byte             `json:"currentHash"`
	BlockBody    *BlockBody         `json:"-"`
	Body         []byte             `json:"body"`
	Round        *i_consensus.Round `json:"round"`
	Locker       sync.RWMutex       `json:"-"`
	StatTree     *MPTPlus.MTP       `json:"-"`
	StatRoot     []byte             `json:"statRoot"`
	TxTree       *MPTPlus.MTP       `json:"-"`
	TxRoot       []byte             `json:"txRoot"`
	EventTree    *MPTPlus.MTP       `json:"-"`
	EventRoot    []byte             `json:"eventRoot"`
}

func (block *Block) String() string {
	block.UpdateMPTPlusRoot()
	return fmt.Sprintf(`{"height": %d, "statRoot": "%s", "txRoot": "%s", "eventRoot": "%s", "body": "%s", nonce": %d, "previousHash": "%s", "round": %s}`,
		block.Height, block.StatRoot, block.TxRoot, block.EventRoot, block.Body, block.Nonce, block.PreviousHash, block.Round.String())
}

func (block *Block) Bytes() []byte {
	return []byte(block.String())
}

func (block *Block) Hash() []byte {
	return block.CurrentHash
}

func (block *Block) CaculateHash() []byte {
	block.CurrentHash = crypto.Sha3_256(block.Bytes())
	return block.CurrentHash
}

func (block *Block) NewNonce() {
	block.Nonce++
}

func (block Block) Validate() error {
	if !bytes.Equal(block.CaculateHash(), block.CurrentHash) {
		return errors.New("Invalid Hash")
	}
	return nil
}

func (block *Block) GetAccount(address []byte) (*common.Account, error) {
	value, err := block.StatTree.GetValue(address)
	if err != nil {
		return nil, err
	}
	var account common.Account
	err = json.Unmarshal(value, &account)
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (block *Block) ExistAddress(address []byte) bool {
	return block.StatTree.ContainsKey(address)
}

func (block *Block) CreateAccount(address, pubKey []byte) {
	if !block.ExistAddress(address) {
		block.newAccount(address, pubKey)
	}
}

func (block *Block) newAccount(address []byte, pubKey []byte) {
	account := common.NewAccount(address, pubKey)
	value, _ := json.Marshal(account)
	block.StatTree.MustInsert(address, value)
	block.UpdateMPTPlusRoot()
}

func (block *Block) NewTransaction(tx *common.Transaction, fee int64) {
	block.Locker.Lock()
	defer block.Locker.Unlock()
	fromAddress, _ := hex.DecodeString(tx.From)
	toAddress, _ := hex.DecodeString(tx.To)
	account, _ := block.GetAccount(fromAddress)
	recieverAccount, _ := block.GetAccount(toAddress)
	var txResult *common.TxResult
	if account.GetAmount() < tx.Amount+block.Fee {
		txResult = common.NewTransactionResult(tx, fee, false, "no enough amount")
	} else {
		txResult = common.NewTransactionResult(tx, fee, true, "")
		account.ReduceAmount(tx.Amount + block.Fee)
		block.TotalFee += block.Fee
		recieverAccount.AddAmount(tx.Amount)
		block.StatTree.MustInsert(fromAddress, account.ToBytes())
		block.StatTree.MustInsert(toAddress, recieverAccount.ToBytes())
	}
	txId, _ := hex.DecodeString(tx.TransactionId())
	block.BlockBody.AddTxResult(*txResult)
	block.TxTree.MustInsert(txId, txResult.ToBytes())
	block.UpdateMPTPlusRoot()
}

func (block *Block) UpdateMPTPlusRoot() {
	block.StatRoot = block.StatTree.Root
	block.TxRoot = block.TxTree.Root
	block.EventRoot = block.EventTree.Root
	block.CaculateHash()
}

func FromBytes2Block(data []byte) (*Block, error) {
	var block Block
	err := json.Unmarshal(data, block)
	block.EventTree = MPTPlus.MTP_Tree(db.GetDBInst(), block.EventRoot)
	block.StatTree = MPTPlus.MTP_Tree(db.GetDBInst(), block.StatRoot)
	block.TxTree = MPTPlus.MTP_Tree(db.GetDBInst(), block.TxRoot)
	block.Locker = sync.RWMutex{}
	if err != nil {
		return nil, err
	}
	return &block, nil
}

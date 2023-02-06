package pow

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

type Tinycoin struct {
	Blocks     []Block
	Pool       TxPool
	Wallet     Wallet
	Difficulty uint
	StopFlg    bool
}

func (tc *Tinycoin) LatestBlock() Block {
	len := len(tc.Blocks)
	return tc.Blocks[len-1]
}

func (tc *Tinycoin) AddBlock(newBlock Block) {
	tc.validBlock(newBlock)
	tc.Blocks = append(tc.Blocks, newBlock)
}

func (tc *Tinycoin) validBlock(block Block) {
	preBlock := tc.LatestBlock()
	expHash := HashBlock(block.Height, block.PreHash, block.Timestamp, block.Data, block.Nonce)

	if preBlock.Height+1 != block.Height {
		panic(fmt.Sprintf("Invalid height. expected: %v", preBlock.Height+1))
	} else if preBlock.Hash != block.PreHash {
		panic(fmt.Sprintf("Invalid preHash. expected: %v", preBlock.Hash))
	} else if expHash.String() != block.Hash {
		panic(fmt.Sprintf("Invalid hash. expected: %v", expHash))
	}

	ok := checkHash(block, tc.Difficulty)
	if !ok {
		panic(fmt.Sprintf("Invalid hash. expected to start from: %v", strings.Repeat("0", int(tc.Difficulty))))
	}
}

func (tc *Tinycoin) GenNextBlock() Block {
	var nonce uint = 0
	pre := tc.LatestBlock()
	coinbaseTx := tc.GenCoinbaseTx()

	ticker := time.NewTicker(1 * time.Second / 32)
	done := make(chan bool)

	var block = Block{}

	go func() {
		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				data := ""
				block = Block{
					Height:    pre.Height + 1,
					PreHash:   pre.Hash,
					Timestamp: time.Now(),
					Data:      data,
					Nonce:     nonce,
				}

				ok := checkHash(block, tc.Difficulty)
				if ok {
					spentTxs := tc.Pool.txs
					emptyPool := make([]Transaction, len(tc.Pool.txs))
					tc.Pool.txs = emptyPool
					tc.Pool.UpdateUnspentTxs(spentTxs)
					tc.Pool.unspentTxs = append(tc.Pool.unspentTxs, coinbaseTx)
					done <- true
				}
				nonce += 1
				fmt.Println("Tick at", t)
			}
		}
	}()

	// time.Sleep(1 * time.Second / 32)
	// ticker.Stop()

	return block
}

func checkHash(block Block, difficulty uint) bool {
	for i, val := range block.Hash {
		if val != rune(0) {
			return false
		}

		if uint(i)+1 > difficulty {
			break
		}
	}
	return true
}

func (tc *Tinycoin) StartMining() {
	for {
		if tc.StopFlg {
			break
		}
		block := tc.GenNextBlock()
		tc.AddBlock(block)
		fmt.Printf("new block mined! block number is %d", block.Height)
	}
}

func (tc *Tinycoin) GenCoinbaseTx() Transaction {
	tx := Transaction{}
	return tc.Wallet.SignTx(tx.NewTransaction("", tc.Wallet.PubKey))
}

type Block struct {
	Height    uint
	PreHash   string
	Timestamp time.Time
	Data      string
	Nonce     uint
	Hash      string
}

func HashBlock(height uint, preHash string, timestamp time.Time, data string, nonce uint) common.Hash {
	return crypto.Keccak256Hash([]byte(fmt.Sprintf("%v,%v,%v,%v,%v", height, preHash, timestamp, data, nonce)))
}

type Transaction struct {
	InHash  string `json:"InHash"`
	InSig   string `json:"InSig"`
	OutAddr string `json:"OutAddr"`
	Hash    string `json:"Hash"`
}

func (t *Transaction) NewTransaction(inHash string, outAddr string) Transaction {
	return Transaction{
		InHash:  inHash,
		OutAddr: outAddr,
		InSig:   "",
		Hash:    HashTransaction(inHash, outAddr).String(),
	}
}

func (t *Transaction) String() string {
	txBytes, err := json.Marshal(t)
	if err != nil {
		log.Fatal(err)
	}
	return string(txBytes)
}

func HashTransaction(inHash string, outAddr string) common.Hash {
	return crypto.Keccak256Hash([]byte(fmt.Sprintf("%v,%v", inHash, outAddr)))
}

type TxPool struct {
	txs        []Transaction
	unspentTxs []Transaction
}

func (tp *TxPool) AddTx(newTx Transaction) {
	tp.ValidateTx(tp.unspentTxs, newTx)
	tp.txs = append(tp.txs, newTx)
}

func (tp *TxPool) BalanceOf(address string) int {
	var tempTxs []Transaction

	for _, unspentTx := range tp.unspentTxs {
		if unspentTx.OutAddr == address {
			tempTxs = append(tempTxs, unspentTx)
		}
	}

	return len(tempTxs)
}

func (tp *TxPool) UpdateUnspentTxs(spentTxs []Transaction) {
	for _, spentTx := range spentTxs {
		// check tx was spent
		var index = -1
		for i, unspentTx := range tp.unspentTxs {
			if unspentTx.Hash == spentTx.InHash {
				index = i
			}
		}

		if index == -1 {
			return
		}

		// remove from unspent txs
		tp.unspentTxs = append(tp.unspentTxs[:index], tp.unspentTxs[index+1:]...)
	}

	tp.unspentTxs = append(tp.unspentTxs, spentTxs...)
}

func (tp *TxPool) ValidateTx(unspentTxs []Transaction, tx Transaction) {
	// check hash value
	if tx.Hash != HashTransaction(tx.InHash, tx.OutAddr).String() {
		panic(fmt.Sprintf("Invalid hash. expected: %v", HashTransaction(tx.InHash, tx.OutAddr)))
	}

	// check tx whether already spent
	var found = false
	var exTx Transaction
	for _, unspentTx := range unspentTxs {
		if unspentTx.Hash == tx.InHash {
			exTx = unspentTx
			found = true
			break
		}
	}

	if !found {
		panic(fmt.Sprintf("Tx is not found"))
	}

	// check signature is valid
	tp.ValidateSig(tx, exTx.OutAddr)
}

func (tp *TxPool) ValidateSig(tx Transaction, address string) bool {
	publicKeyBytes, err := hexutil.Decode(address)
	if err != nil {
		fmt.Printf("Fail to decode address")
		return false
	}

	pubKey, err := crypto.ToECDSA(publicKeyBytes)
	if err != nil {
		fmt.Printf("Fail to ECDSA")
		return false
	}
	return ecdsa.VerifyASN1(&pubKey.PublicKey, []byte(tx.Hash), []byte(tx.InSig))
}

type Wallet struct {
	PriKey ecdsa.PrivateKey `json:"PriKey"`
	PubKey string           `json:"PubKey"`
}

func (w *Wallet) New() {
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		fmt.Printf("%v", err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("cannot assert type: publicKey is not of type *esdsa.PublicKey")
	}

	publicKeyBytes := crypto.FromECDSAPub(publicKeyECDSA)

	w.PriKey = *privateKey
	w.PubKey = hexutil.Encode(publicKeyBytes)
}

func (w *Wallet) SignTx(tx Transaction) Transaction {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	msg := "hello, world"
	hash := sha256.Sum256([]byte(msg))

	sig, err := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
	if err != nil {
		panic(err)
	}
	tx.InSig = string(sig)

	return tx
	// fmt.Printf("signature: %x\n", sig)

}

// func toHexString(bytes []byte) string {

// }

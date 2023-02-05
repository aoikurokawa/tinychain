package pow

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
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
	expHash := HashBlock(block.height, block.preHash, block.timestamp, block.data, block.nonce)

	if preBlock.height+1 != block.height {
		panic(fmt.Sprintf("Invalid height. expected: %v", preBlock.height+1))
	} else if preBlock.hash != block.preHash {
		panic(fmt.Sprintf("Invalid preHash. expected: %v", preBlock.hash))
	} else if expHash.String() != block.hash {
		panic(fmt.Sprintf("Invalid hash. expected: %v", expHash))
	}

	for i, val := range block.hash {
		if val != rune(0) {
			panic(fmt.Sprintf("Invalid hash. expected to start from: %v", strings.Repeat("0", int(tc.Difficulty))))
		}

		if uint(i)+1 > tc.Difficulty {
			break
		}
	}
}

func (tc *Tinycoin) GenNextBlock() Block {
	nonce := 0
	pre := tc.LatestBlock()
	conbaseTx := tc.GenCoinbaseTx()

	ticker := time.NewTicker(1 * time.Second / 32)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				data := ""
				for _, tx := range tc.pool.txs {
					fmt.Sprintf("%v", tx)
				}
				fmt.Println("Tick at", t)
			}
		}
	}()

	time.Sleep(1 * time.Second / 32)
	ticker.Stop()
	done <- true
	// fmt.Println("Ticker stopped")
	return Block{}
}

func (tc *Tinycoin) StartMining() {
	for {
		if tc.StopFlg {
			break
		}
		block := tc.GenNextBlock()
		tc.AddBlock(block)
		fmt.Printf("new block mined! block number is %d", block.height)
	}
}

func (tc *Tinycoin) GenCoinbaseTx() Transaction {
	tx := Transaction{}
	return tc.Wallet.SignTx(tx.NewTransaction("", tc.Wallet.pubKey))
}

type Block struct {
	height    uint
	preHash   string
	timestamp uint
	data      string
	nonce     uint
	hash      string
}

func HashBlock(height uint, preHash string, timestamp uint, data string, nonce uint) common.Hash {
	return crypto.Keccak256Hash([]byte(fmt.Sprintf("%v,%v,%v,%v,%v", height, preHash, timestamp, data, nonce)))
}

type Transaction struct {
	InHash  string
	InSig   string
	OutAddr string
	Hash    string
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
	return fmt.Sprintf("%v, %v, %v, %v", t.InHash, t.OutAddr, t.InSig, t.Hash)
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
	priKey ecdsa.PrivateKey
	pubKey string
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

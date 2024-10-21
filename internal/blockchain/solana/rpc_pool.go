// pkg/blockchain/solana/rpc_pool.go
package solana

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
)

type RPCPool struct {
	clients []*rpc.Client
	mutex   sync.Mutex
	index   int
}

func NewRPCPool(rpcList []string) *RPCPool {
	// Создаем список RPC-клиентов из rpcList
	var clients []*rpc.Client
	for _, url := range rpcList {
		client := rpc.New(url)
		clients = append(clients, client)
	}

	return &RPCPool{
		clients: clients,
		index:   0,
	}
}

func (p *RPCPool) GetClient() *rpc.Client {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Возвращаем следующий доступный RPC-клиент (круговой цикл)
	client := p.clients[p.index]
	p.index = (p.index + 1) % len(p.clients)
	return client
}

func (p *RPCPool) CheckClientHealth(client *rpc.Client) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.GetRecentBlockhash(ctx, rpc.CommitmentFinalized)
	return err == nil
}

func (p *RPCPool) PerformHealthChecks() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for i, client := range p.clients {
		if !p.CheckClientHealth(client) {
			log.Printf("RPC клиент %d недоступен, удаляем из пула", i)
			p.clients = append(p.clients[:i], p.clients[i+1:]...)
		}
	}
}

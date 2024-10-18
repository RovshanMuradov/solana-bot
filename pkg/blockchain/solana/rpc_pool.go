package solana

import (
	"sync"

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

package eventlistener

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockWSServer struct {
	server   *httptest.Server
	handler  func(conn net.Conn)
	conns    []net.Conn
	connLock sync.Mutex
}

func newMockWSServer(handler func(conn net.Conn)) *mockWSServer {
	mock := &mockWSServer{
		handler: handler,
		conns:   make([]net.Conn, 0),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}

		mock.connLock.Lock()
		mock.conns = append(mock.conns, conn)
		mock.connLock.Unlock()

		go mock.handler(conn)
	}))

	return mock
}

func (m *mockWSServer) Close() {
	m.server.Close()
	m.connLock.Lock()
	defer m.connLock.Unlock()
	for _, conn := range m.conns {
		conn.Close()
	}
}

func (m *mockWSServer) URL() string {
	return "ws" + strings.TrimPrefix(m.server.URL, "http")
}

func TestEventListener_EventHandling(t *testing.T) {
	testCases := []struct {
		name     string
		event    Event
		expected bool
	}{
		{
			name: "Valid NewPool event",
			event: Event{
				Type:   "NewPool",
				PoolID: "pool123",
				TokenA: "SOL",
				TokenB: "USDC",
			},
			expected: true,
		},
		{
			name: "Valid PriceChange event",
			event: Event{
				Type:   "PriceChange",
				PoolID: "pool123",
				TokenA: "SOL",
				TokenB: "USDC",
			},
			expected: true,
		},
		{
			name: "Invalid event type",
			event: Event{
				Type: "InvalidType",
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			eventReceived := make(chan Event, 1)

			mock := newMockWSServer(func(conn net.Conn) {
				time.Sleep(100 * time.Millisecond)
				eventJSON, _ := json.Marshal(tc.event)
				err := wsutil.WriteServerText(conn, eventJSON)
				require.NoError(t, err)
			})
			defer mock.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
			require.NoError(t, err)

			err = listener.Subscribe(ctx, func(event Event) {
				select {
				case eventReceived <- event:
				case <-ctx.Done():
				}
			})
			require.NoError(t, err)

			if tc.expected {
				select {
				case receivedEvent := <-eventReceived:
					assert.Equal(t, tc.event.Type, receivedEvent.Type)
					assert.Equal(t, tc.event.PoolID, receivedEvent.PoolID)
					assert.Equal(t, tc.event.TokenA, receivedEvent.TokenA)
					assert.Equal(t, tc.event.TokenB, receivedEvent.TokenB)
				case <-time.After(1 * time.Second):
					t.Fatal("Timeout waiting for event")
				}
			} else {
				select {
				case <-eventReceived:
					t.Error("Received unexpected event")
				case <-time.After(500 * time.Millisecond):
					// Expected behavior for invalid event
				}
			}
		})
	}
}

func TestEventListener_Reconnection(t *testing.T) {
	eventReceived := make(chan Event, 1)
	connectionCount := 0
	var connMutex sync.Mutex

	mock := newMockWSServer(func(conn net.Conn) {
		connMutex.Lock()
		connectionCount++
		currentCount := connectionCount
		connMutex.Unlock()

		t.Logf("Connection handler called, count: %d", currentCount)

		if currentCount == 1 {
			t.Log("First connection - closing")
			conn.Close()
			return
		}

		t.Log("Second connection - sending event")
		event := Event{
			Type:   "NewPool",
			PoolID: "reconnected_pool",
			TokenA: "SOL",
			TokenB: "USDC",
		}
		eventJSON, _ := json.Marshal(event)
		err := wsutil.WriteServerText(conn, eventJSON)
		require.NoError(t, err)
		if err != nil {
			t.Logf("Error sending event: %v", err)
		} else {
			t.Log("Event sent successfully")
		}
	})
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Log("Creating new event listener")
	listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
	require.NoError(t, err)

	t.Log("Subscribing to events")
	err = listener.Subscribe(ctx, func(event Event) {
		t.Log("Handler received event")
		select {
		case eventReceived <- event:
			t.Log("Event sent to channel")
		case <-ctx.Done():
			t.Log("Context done while handling event")
		}
	})
	require.NoError(t, err)

	t.Log("Waiting for event")
	select {
	case event := <-eventReceived:
		t.Logf("Received event: %+v", event)
		assert.Equal(t, "NewPool", event.Type)
		assert.Equal(t, "reconnected_pool", event.PoolID)
		assert.Equal(t, "SOL", event.TokenA)
		assert.Equal(t, "USDC", event.TokenB)
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for event after reconnection")
	}
}

func TestEventListener_InvalidMessage(t *testing.T) {
	eventsReceived := make(chan Event, 2)
	messagesSent := make(chan struct{})

	mock := newMockWSServer(func(conn net.Conn) {
		messages := []struct {
			data  []byte
			delay time.Duration
		}{
			{
				data:  []byte("{invalid json}"),
				delay: 50 * time.Millisecond,
			},
			{
				data:  []byte(`{"type":"NewPool","pool_id":"valid_pool","token_a":"SOL","token_b":"USDC"}`),
				delay: 50 * time.Millisecond,
			},
		}

		for _, msg := range messages {
			err := wsutil.WriteServerMessage(conn, ws.OpText, msg.data)
			require.NoError(t, err)
			time.Sleep(msg.delay)
		}
		close(messagesSent)
	})
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
	require.NoError(t, err)

	messageCount := 0
	err = listener.Subscribe(ctx, func(event Event) {
		messageCount++
		select {
		case eventsReceived <- event:
		case <-time.After(100 * time.Millisecond):
			t.Error("Failed to send event to channel")
		}
	})
	require.NoError(t, err)

	<-messagesSent

	select {
	case event := <-eventsReceived:
		assert.Equal(t, "NewPool", event.Type)
		assert.Equal(t, "valid_pool", event.PoolID)
		assert.Equal(t, "SOL", event.TokenA)
		assert.Equal(t, "USDC", event.TokenB)
	case <-ctx.Done():
		t.Fatal("Timeout waiting for valid event after invalid message")
	}

	assert.Equal(t, 1, messageCount)
}

func TestEventListener_MultipleMessageTypes(t *testing.T) {
	eventsReceived := make(chan Event, 5)
	messagesSent := make(chan struct{})

	mock := newMockWSServer(func(conn net.Conn) {
		messages := [][]byte{
			[]byte("{invalid json}"),
			[]byte(`{"type":"InvalidType"}`),
			[]byte(`{"type":"NewPool","pool_id":"pool1","token_a":"SOL","token_b":"USDC"}`),
			[]byte(`{"type":"PriceChange","pool_id":"pool1","token_a":"SOL","token_b":"USDC"}`),
			[]byte(`{"type":""}`),
		}

		for _, msg := range messages {
			err := wsutil.WriteServerMessage(conn, ws.OpText, msg)
			require.NoError(t, err)
			time.Sleep(50 * time.Millisecond)
		}
		close(messagesSent)
	})
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
	require.NoError(t, err)

	receivedCount := 0
	err = listener.Subscribe(ctx, func(event Event) {
		receivedCount++
		select {
		case eventsReceived <- event:
		case <-time.After(100 * time.Millisecond):
			t.Error("Failed to send event to channel")
		}
	})
	require.NoError(t, err)

	<-messagesSent

	expectedEvents := []struct {
		eventType string
		poolID    string
	}{
		{"NewPool", "pool1"},
		{"PriceChange", "pool1"},
	}

	for _, expected := range expectedEvents {
		select {
		case event := <-eventsReceived:
			assert.Equal(t, expected.eventType, event.Type)
			assert.Equal(t, expected.poolID, event.PoolID)
			assert.Equal(t, "SOL", event.TokenA)
			assert.Equal(t, "USDC", event.TokenB)
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for event type %s", expected.eventType)
		}
	}

	assert.Equal(t, len(expectedEvents), receivedCount)
}

func TestEventListener_Close(t *testing.T) {
	mock := newMockWSServer(func(_ net.Conn) {
		// Keep connection open
		select {}
	})
	defer mock.Close()

	ctx := context.Background()
	listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
	require.NoError(t, err)

	done := make(chan struct{})
	err = listener.Subscribe(ctx, func(_ Event) {})
	require.NoError(t, err)

	go func() {
		listener.Close()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for listener to close")
	}
}

func TestEventListener_MultipleConcurrentConnections(t *testing.T) {
	const numConnections = 10
	eventReceived := make(chan Event, numConnections)
	var wg sync.WaitGroup
	errorCh := make(chan error, numConnections)

	mock := newMockWSServer(func(conn net.Conn) {
		event := Event{
			Type:   "NewPool",
			PoolID: "concurrent_pool",
			TokenA: "SOL",
			TokenB: "USDC",
		}
		eventJSON, _ := json.Marshal(event)
		err := wsutil.WriteServerText(conn, eventJSON)
		if err != nil {
			fmt.Printf("Failed to write event: %v\n", err)
		}
	})
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
			if err != nil {
				errorCh <- fmt.Errorf("failed to create listener: %w", err)
				return
			}

			err = listener.Subscribe(ctx, func(event Event) {
				eventReceived <- event
			})
			if err != nil {
				errorCh <- fmt.Errorf("failed to subscribe: %w", err)
				return
			}

			<-ctx.Done()
			listener.Close()
		}()
	}

	for i := 0; i < numConnections; i++ {
		select {
		case err := <-errorCh:
			t.Fatalf("Error in goroutine: %v", err)
		case event := <-eventReceived:
			assert.Equal(t, "NewPool", event.Type)
			assert.Equal(t, "concurrent_pool", event.PoolID)
			assert.Equal(t, "SOL", event.TokenA)
			assert.Equal(t, "USDC", event.TokenB)
		case <-time.After(15 * time.Second):
			t.Fatalf("Timeout waiting for event %d", i+1)
		}
	}

	cancel()
	wg.Wait()
}

func BenchmarkEventListener_HandleHighLoad(b *testing.B) {
	mock := newMockWSServer(func(conn net.Conn) {
		for i := 0; i < b.N; i++ {
			event := Event{
				Type:   "PriceChange",
				PoolID: fmt.Sprintf("pool%d", i),
				TokenA: "SOL",
				TokenB: "USDC",
			}
			eventJSON, _ := json.Marshal(event)
			err := wsutil.WriteServerText(conn, eventJSON)
			require.NoError(b, err)

		}
	})
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
	require.NoError(b, err)

	err = listener.Subscribe(ctx, func(_ Event) {})
	require.NoError(b, err)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Параллельная обработка событий
		}
	})
}

func TestEventListener_SuddenConnectionClose(t *testing.T) {
	eventReceived := make(chan Event, 1)

	mock := newMockWSServer(func(conn net.Conn) {
		event := Event{
			Type:   "NewPool",
			PoolID: "sudden_close_pool",
			TokenA: "SOL",
			TokenB: "USDC",
		}
		eventJSON, _ := json.Marshal(event)
		err := wsutil.WriteServerText(conn, eventJSON)
		require.NoError(t, err)

		conn.Close() // Внезапно закрываем соединение
	})
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
	require.NoError(t, err)

	err = listener.Subscribe(ctx, func(event Event) {
		eventReceived <- event
	})
	require.NoError(t, err)

	select {
	case event := <-eventReceived:
		assert.Equal(t, "NewPool", event.Type)
		assert.Equal(t, "sudden_close_pool", event.PoolID)
		assert.Equal(t, "SOL", event.TokenA)
		assert.Equal(t, "USDC", event.TokenB)
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for event after sudden connection close")
	}
}

func TestEventListener_MessageDelays(t *testing.T) {
	eventReceived := make(chan Event, 1)

	mock := newMockWSServer(func(conn net.Conn) {
		time.Sleep(2 * time.Second) // Задержка перед отправкой события
		event := Event{
			Type:   "PriceChange",
			PoolID: "delayed_pool",
			TokenA: "SOL",
			TokenB: "USDC",
		}
		eventJSON, _ := json.Marshal(event)
		err := wsutil.WriteServerText(conn, eventJSON)
		require.NoError(t, err)
	})
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
	require.NoError(t, err)

	err = listener.Subscribe(ctx, func(event Event) {
		eventReceived <- event
	})
	require.NoError(t, err)

	select {
	case event := <-eventReceived:
		assert.Equal(t, "PriceChange", event.Type)
		assert.Equal(t, "delayed_pool", event.PoolID)
		assert.Equal(t, "SOL", event.TokenA)
		assert.Equal(t, "USDC", event.TokenB)
	case <-time.After(4 * time.Second):
		t.Fatal("Timeout waiting for delayed event")
	}
}

func TestEventListener_PartialFields(t *testing.T) {
	eventReceived := make(chan Event, 1)

	mock := newMockWSServer(func(conn net.Conn) {
		event := Event{
			Type: "NewPool",
			// PoolID отсутствует
			TokenA: "SOL",
			TokenB: "USDC",
		}
		eventJSON, _ := json.Marshal(event)
		err := wsutil.WriteServerText(conn, eventJSON)
		require.NoError(t, err)
	})
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
	require.NoError(t, err)

	err = listener.Subscribe(ctx, func(event Event) {
		eventReceived <- event
	})
	require.NoError(t, err)

	select {
	case <-eventReceived:
		t.Error("Received event with missing fields, expected to be ignored")
	case <-time.After(1 * time.Second):
		// Ожидаемое поведение: событие игнорируется
	}
}

// func TestEventListener_ExtraFields(t *testing.T) {
// 	eventReceived := make(chan Event, 1)

// 	mock := newMockWSServer(func(conn net.Conn) {
// 		eventJSON := `{"type":"NewPool","pool_id":"extra_field_pool","token_a":"SOL","token_b":"USDC","extra_field":"extra_value"}`
// 		err := wsutil.WriteServerText(conn, []byte(eventJSON))
// 		if err != nil {
// 			t.Logf("Failed to write event: %v", err)
// 		}
// 	})

// 	defer mock.Close()

// 	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
// 	defer cancel()

// 	listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
// 	require.NoError(t, err)

// 	err = listener.Subscribe(ctx, func(event Event) {
// 		eventReceived <- event
// 	})
// 	require.NoError(t, err)

// 	select {
// 	case event := <-eventReceived:
// 		assert.Equal(t, "NewPool", event.Type)
// 		assert.Equal(t, "extra_field_pool", event.PoolID)
// 		assert.Equal(t, "SOL", event.TokenA)
// 		assert.Equal(t, "USDC", event.TokenB)
// 	case <-time.After(3 * time.Second):
// 		t.Fatal("Timeout waiting for event with extra fields")
// 	}

// }

func TestEventListener_NoLeakAfterClose(t *testing.T) {
	mock := newMockWSServer(func(conn net.Conn) {
		// Читаем из соединения, чтобы обнаружить его закрытие
		defer conn.Close()
		buf := make([]byte, 1024)
		for {
			_, err := conn.Read(buf)
			if err != nil {
				if err == io.EOF {
					t.Log("Connection closed by client")
				} else {
					t.Logf("Error reading from connection: %v", err)
				}
				break
			}
		}
	})
	defer mock.Close()

	ctx := context.Background()
	listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
	require.NoError(t, err)

	err = listener.Subscribe(ctx, func(_ Event) {})
	require.NoError(t, err)

	listener.Close()

	// Даем время соединению закрыться
	time.Sleep(100 * time.Millisecond)

	// Проверка, что соединение закрыто
	mock.connLock.Lock()
	defer mock.connLock.Unlock()
	for _, conn := range mock.conns {
		_, err := conn.Write([]byte("ping"))
		assert.Error(t, err, "Connection should be closed")
	}
}

func TestEventListener_InvalidAuthentication(t *testing.T) {
	mock := newMockWSServer(func(conn net.Conn) {
		// Отправляем ошибку аутентификации
		errorMessage := `{"error":"authentication_failed"}`
		err := wsutil.WriteServerText(conn, []byte(errorMessage))
		require.NoError(t, err)

		conn.Close()
	})
	defer mock.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
	require.NoError(t, err)

	err = listener.Subscribe(ctx, func(_ Event) {})
	require.NoError(t, err)

	// Ожидаем, что слушатель не получит никаких событий
	select {
	case <-time.After(1 * time.Second):
		// Ожидаемо
	case <-listener.done:
		// Возможно, слушатель закрылся из-за ошибки
	}
}

// func TestEventListener_Fuzzing(t *testing.T) {
// 	eventsReceived := make(chan Event, 100)

// 	mock := newMockWSServer(func(conn net.Conn) {
// 		for i := 0; i < 100; i++ {
// 			// Генерируем случайный JSON
// 			randomJSON := fmt.Sprintf(`{"type":"%d", "data":"%s"}`, i, "random")
// 			err := wsutil.WriteServerText(conn, []byte(randomJSON))
// 			require.NoError(t, err)
// 		}
// 	})
// 	defer mock.Close()

// 	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 	defer cancel()

// 	listener, err := NewEventListener(ctx, mock.URL(), zap.NewNop())
// 	require.NoError(t, err)

// 	err = listener.Subscribe(ctx, func(event Event) {
// 		eventsReceived <- event
// 	})
// 	require.NoError(t, err)

// 	// Проверяем, что слушатель не падает и корректно обрабатывает случайные данные
// 	select {
// 	case <-time.After(5 * time.Second):
// 		// Ожидаемо
// 	case event := <-eventsReceived:
// 		// Возможно, некоторые валидные события были получены
// 		t.Logf("Received event: %+v", event)
// 	}
// }

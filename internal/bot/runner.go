// internal/bot/runner.go
package bot

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"github.com/rovshanmuradov/solana-bot/internal/task"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
)

// Runner представляет контроллер основного процесса бота, управляющий
// жизненным циклом приложения, обработкой задач и взаимодействием с блокчейном.
//
// Runner координирует выполнение задач через несколько параллельных воркеров,
// управляет кошельками и обеспечивает корректное завершение работы при получении сигналов.
type Runner struct {
	logger        *zap.Logger
	config        *task.Config
	solClient     *solbc.Client
	taskManager   *task.Manager
	wallets       map[string]*wallet.Wallet
	defaultWallet *wallet.Wallet
	shutdownCh    chan os.Signal
}

// NewRunner создает новый экземпляр Runner с базовой инициализацией.
//
// Метод инициализирует только логгер и канал для сигналов завершения работы.
// Полная настройка компонентов выполняется методом Initialize.
//
// Параметры:
//   - logger: настроенный экземпляр zap.Logger для логирования
//
// Возвращает:
//   - *Runner: новый экземпляр Runner с базовой инициализацией
func NewRunner(logger *zap.Logger) *Runner {
	return &Runner{
		logger:     logger,
		shutdownCh: make(chan os.Signal, 1),
	}
}

// Initialize настраивает все зависимости и компоненты Runner.
//
// Метод выполняет загрузку конфигурации из файла, инициализирует кошельки,
// создает клиент для взаимодействия с блокчейном Solana и настраивает
// менеджер задач. Должен вызываться перед методом Run.
//
// Параметры:
//   - configPath: путь к файлу конфигурации
//
// Возвращает:
//   - error: ошибку, если не удалось инициализировать какой-либо компонент
func (r *Runner) Initialize(configPath string) error {
	r.logger.Info("Initializing bot runner")

	// Load configuration
	cfg, err := task.LoadConfig(configPath)
	if err != nil {
		return err
	}
	r.config = cfg
	r.logger.Sugar().Infof("Config loaded: %+v", cfg)

	// Load wallets
	wallets, err := wallet.LoadWallets("configs/wallets.csv")
	if err != nil {
		return err
	}
	r.wallets = wallets
	r.logger.Info("Wallets loaded", zap.Int("count", len(wallets)))

	// Pick first wallet as default if needed
	for _, w := range wallets {
		r.defaultWallet = w
		break
	}

	// Initialize Solana client
	r.solClient = solbc.NewClient(cfg.RPCList[0], r.logger)

	// Initialize task manager
	r.taskManager = task.NewManager(r.logger)

	return nil
}

// Run запускает основную логику бота с параллельными воркерами.
//
// Метод настраивает обработку сигналов завершения работы, загружает задачи
// из файла и распределяет их между воркерами для параллельного выполнения.
// Количество воркеров определяется из конфигурации. Метод блокирует выполнение
// до завершения всех задач или получения сигнала завершения работы.
//
// Параметры:
//   - ctx: контекст выполнения, используемый для отмены операций
//
// Возвращает:
//   - error: ошибку, если не удалось загрузить задачи или произошла критическая ошибка
func (r *Runner) Run(ctx context.Context) error {
	// Setup signal handling
	signal.Notify(r.shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	// Create a context that can be cancelled
	shutdownCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Setup handler for shutdown signal
	go func() {
		sig := <-r.shutdownCh
		r.logger.Info("Signal received", zap.String("signal", sig.String()))
		cancel()
	}()

	// Load task definitions
	tasks, err := r.taskManager.LoadTasks("configs/tasks.csv")
	if err != nil {
		return err
	}
	r.logger.Info("Tasks loaded", zap.Int("count", len(tasks)))

	// Create a task channel and add tasks to it
	taskCh := make(chan *task.Task, len(tasks))
	for _, t := range tasks {
		taskCh <- t
	}
	close(taskCh)

	// Determine number of workers from config
	numWorkers := r.config.Workers
	if numWorkers <= 0 {
		numWorkers = 1
	}
	r.logger.Info("Starting task execution", zap.Int("workers", numWorkers))

	// Create a wait group to track worker completion
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Start workers
	for i := 0; i < numWorkers; i++ {
		workerID := i + 1
		go func(id int) {
			defer wg.Done()
			r.worker(id, shutdownCtx, taskCh)
		}(workerID)
	}

	// Wait for workers to complete or context to be cancelled
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Wait for workers to finish or context to be cancelled
	select {
	case <-done:
		r.logger.Info("All tasks completed successfully")
	case <-shutdownCtx.Done():
		r.logger.Info("Execution interrupted, waiting for workers to finish")
		// Wait for workers to finish gracefully
		select {
		case <-done:
			r.logger.Info("All workers finished gracefully")
		case <-time.After(5 * time.Second):
			r.logger.Warn("Not all workers finished in time")
		}
	}

	return nil
}

// Shutdown выполняет корректное завершение работы бота.
//
// Метод обеспечивает правильное закрытие всех подсистем и компонентов,
// включая синхронизацию логгера. Игнорирует стандартные ошибки синхронизации
// логгера, которые могут возникать в некоторых окружениях.
func (r *Runner) Shutdown() {
	r.logger.Info("Bot shutting down gracefully")
	// Здесь может быть код для корректного завершения всех подсистем

	// Безопасный вызов Sync для логгера
	if err := r.logger.Sync(); err != nil {
		// Игнорируем стандартные ошибки при синхронизации логгера
		if !os.IsNotExist(err) &&
			err.Error() != "sync /dev/stdout: invalid argument" &&
			err.Error() != "sync /dev/stderr: inappropriate ioctl for device" {
			// Выводим ошибку напрямую, так как логгер может уже не работать
			fmt.Fprintf(os.Stderr, "failed to sync logger during shutdown: %v\n", err)
		}
	}
}

// WaitForShutdown блокирует выполнение до получения сигнала завершения работы.
//
// Метод ожидает сигналы SIGINT или SIGTERM, после чего вызывает метод Shutdown
// для корректного завершения работы. Обычно вызывается из основного потока
// приложения после запуска бота.
func (r *Runner) WaitForShutdown() {
	sig := <-r.shutdownCh
	r.logger.Info("Signal received", zap.String("signal", sig.String()))
	r.Shutdown()
}

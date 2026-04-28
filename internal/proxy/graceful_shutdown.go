package proxy

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// GracefulShutdown 优雅退出管理器
type GracefulShutdown struct {
	server          *http.Server
	storage         StorageCloser
	streamWg        sync.WaitGroup // 流式响应等待组
	shutdownTimeout time.Duration  // 关闭超时
	shuttingDown    bool           // 是否正在关闭
	mu              sync.RWMutex
}

// StorageCloser 存储关闭接口
type StorageCloser interface {
	Close() error
	Flush() []error
}

// NewGracefulShutdown 创建优雅退出管理器
func NewGracefulShutdown(server *http.Server, storage StorageCloser, shutdownTimeout time.Duration) *GracefulShutdown {
	if shutdownTimeout <= 0 {
		shutdownTimeout = 30 * time.Second
	}
	return &GracefulShutdown{
		server:          server,
		storage:         storage,
		shutdownTimeout: shutdownTimeout,
	}
}

// StartStream 开始流式响应（增加等待计数）
func (gs *GracefulShutdown) StartStream() {
	gs.streamWg.Add(1)
}

// EndStream 结束流式响应（减少等待计数）
func (gs *GracefulShutdown) EndStream() {
	gs.streamWg.Done()
}

// IsShuttingDown 检查是否正在关闭
func (gs *GracefulShutdown) IsShuttingDown() bool {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.shuttingDown
}

// Wait 等待关闭信号
func (gs *GracefulShutdown) Wait() {
	// 监听信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 等待信号
	sig := <-sigChan
	LogInfof("收到信号 %v，开始优雅关闭...", sig)

	// 标记正在关闭
	gs.mu.Lock()
	gs.shuttingDown = true
	gs.mu.Unlock()

	// 执行关闭流程
	gs.shutdown()
}

// shutdown 执行关闭流程
func (gs *GracefulShutdown) shutdown() {
	startTime := time.Now()

	// 1. 停止接收新请求
	LogInfof("[1/4] 停止接收新请求...")
	ctx, cancel := context.WithTimeout(context.Background(), gs.shutdownTimeout)
	defer cancel()

	if err := gs.server.Shutdown(ctx); err != nil {
		LogErrorf("停止服务器失败: %v", err)
	}

	// 2. 等待所有流式响应完成
	LogInfof("[2/4] 等待流式响应完成...")
	done := make(chan struct{})
	go func() {
		gs.streamWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		LogInfof("所有流式响应已完成")
	case <-time.After(gs.shutdownTimeout):
		LogErrorf("等待流式响应超时，强制关闭")
	}

	// 3. 强制刷盘所有映射
	LogInfof("[3/4] 强制刷盘所有映射...")
	if gs.storage != nil {
		errors := gs.storage.Flush()
		if len(errors) > 0 {
			LogErrorf("刷盘失败: %v", errors)
		} else {
			LogInfof("刷盘完成")
		}
	}

	// 4. 关闭存储
	LogInfof("[4/4] 关闭存储...")
	if gs.storage != nil {
		if err := gs.storage.Close(); err != nil {
			LogErrorf("关闭存储失败: %v", err)
		} else {
			LogInfof("存储已关闭")
		}
	}

	elapsed := time.Since(startTime)
	LogInfof("优雅关闭完成，耗时 %v", elapsed)
}

// ShutdownChan 返回关闭通道（用于外部监听）
func (gs *GracefulShutdown) ShutdownChan() <-chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	return sigChan
}

// ManualShutdown 手动触发关闭（用于测试或管理接口）
func (gs *GracefulShutdown) ManualShutdown() {
	gs.mu.Lock()
	gs.shuttingDown = true
	gs.mu.Unlock()
	gs.shutdown()
}

// ============ 辅助函数 ============

// LogInfof 日志函数
func shutdownLogInfof(format string, args ...interface{}) {
	fmt.Printf("[INFO] "+format+"\n", args...)
}

// LogErrorf 日志函数
func shutdownLogErrorf(format string, args ...interface{}) {
	fmt.Printf("[ERROR] "+format+"\n", args...)
}

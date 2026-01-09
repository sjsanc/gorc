package tests

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/sjsanc/gorc/manager"
	"github.com/sjsanc/gorc/runtime"
	"github.com/sjsanc/gorc/storage"
	"github.com/sjsanc/gorc/worker"
	"go.uber.org/zap"
)

func TestIntegration(t *testing.T) {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	sugar := logger.Sugar()

	addr := "0.0.0.0"
	port := 5555
	storageType := storage.StorageInMemory
	runtimeType := runtime.RuntimeDocker

	m, err := manager.NewManager(sugar, addr, port, storageType, runtimeType)
	if err != nil {
		t.Fatalf("Error creating Manager: %s", err)
	}

	go m.Run()

	w, err := worker.NewWorker(sugar, addr, 5556, "0.0.0.0:5555")
	if err != nil {
		t.Fatalf("Error creating Worker: %s", err)
	}

	go w.Run()

	time.Sleep(3 * time.Second)

	resp, err := http.Get("http://0.0.0.0:5555/worker")
	if err != nil {
		t.Fatalf("Error getting nodes: %s", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Error reading response body: %s", err)
	}
	t.Logf("Response Body:\n%s", string(body))
}

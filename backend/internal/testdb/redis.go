package testdb

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// UseRedis isolates runtime counters and leases for a nonparallel service test.
func UseRedis(t *testing.T) {
	t.Helper()
	binary, err := exec.LookPath("redis-server")
	if err != nil {
		t.Fatal("redis-server is required for Studio service tests")
	}
	dir, err := os.MkdirTemp("", "studio-redis-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "r.sock")
	cmd := exec.Command(binary, "--port", "0", "--unixsocket", socket, "--unixsocketperm", "700", "--save", "", "--appendonly", "no", "--dir", dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: socket, MaxRetries: -1})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for client.Ping(ctx).Err() != nil {
		if ctx.Err() != nil {
			t.Fatal("isolated Redis failed to start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Setenv("REDIS_URL", "unix://"+socket)
}

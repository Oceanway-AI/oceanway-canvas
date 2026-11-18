package app

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
	"yingce/backend/internal/testdb"

	"gorm.io/gorm"
)

// newPostgresTestDB creates a schema isolated from every other test.
func newPostgresTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := testdb.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	return db
}

func startDeletionTestWorkers(t *testing.T, svc *Service) {
	t.Helper()
	// 只启用生命周期跟踪，不启动生成、支付等无关的周期 worker。
	svc.backgroundWorkers().Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := svc.StopWorker(ctx); err != nil {
			t.Errorf("stop deletion workers: %v", err)
		}
	})
}

func TestPostgresTestDBAllowsConcurrentReadWrite(t *testing.T) {
	db := newPostgresTestDB(t)
	if err := db.Exec("create table lock_probe (id integer primary key, value text)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("insert into lock_probe (id, value) values (1, 'a')").Error; err != nil {
		t.Fatal(err)
	}

	var mutex sync.Mutex
	var failure error
	record := func(err error) {
		if err == nil {
			return
		}
		mutex.Lock()
		defer mutex.Unlock()
		if failure == nil {
			failure = err
		}
	}

	stop := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			var count int64
			record(db.Table("lock_probe").Count(&count).Error)
		}
	}()
	go func() {
		defer workers.Done()
		defer close(stop)
		for index := 0; index < 300; index++ {
			record(db.Exec("update lock_probe set value = ? where id = 1", fmt.Sprint(index)).Error)
			time.Sleep(time.Millisecond)
		}
	}()
	workers.Wait()

	if failure != nil {
		t.Fatalf("并发读写失败：%v", failure)
	}
}

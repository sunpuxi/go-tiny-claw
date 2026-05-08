package main

import (
	"log"
	"time"
)

func main() {
	// 创建一个容量为 5 的带缓冲 channel，作为令牌池
	taskChannel := make(chan struct{}, 5)
	ids := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
	nowTime := time.Now()

	// 启动一个专门的协程，负责向令牌池里投放初始的 5 个令牌
	go func() {
		for i := 0; i < cap(taskChannel); i++ {
			taskChannel <- struct{}{}
		}
	}()

	for _, id := range ids {
		// 【核心修改点】将循环变量 id 作为参数传入，避免闭包捕获陷阱
		go func(taskID int) {
			// 1. 子协程启动后，先尝试从令牌池获取一个令牌。
			// 如果池子空了（说明并发量已达上限），这里会自动阻塞排队，不会导致主协程卡死。
			<-taskChannel

			// 2. 拿到令牌后，执行具体的业务逻辑
			err := doSomething(taskID)
			if err != nil {
				log.Printf("Error in doSomething: %s", err)
			}

			// 3. 业务执行完毕，将令牌放回池中，让排队的其他协程可以获取并执行
			taskChannel <- struct{}{}
		}(id)
	}

	// 注意：在纯 channel 且没有 WaitGroup 的情况下，主协程无法准确知道所有任务何时完成。
	// 这里只能用 time.Sleep 简单模拟等待（实际工程中通常还是会配合 WaitGroup 来优雅退出）
	time.Sleep(15 * time.Second)
	log.Printf("Main goroutine exiting... cost %+v", time.Since(nowTime))
}

func doSomething(id int) error {
	log.Printf("[doSomething] start doing something of %d", id)
	time.Sleep(2 * time.Second)
	log.Printf("[doSomething] finish doing something of %d", id)
	return nil
}

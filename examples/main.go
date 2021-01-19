package main

import (
	"context"
	"fmt"
	client "github.com/bakurin/payyo-sdk-go-client"
	"io"
	"log"
	"net/http"
	"time"
)

type merchantRequest struct {
	MerchantID int `json:"merchant_id"`
}

type merchantDetails struct {
	MerchantID int    `json:"merchant_id"`
	Name       string `json:"name"`
}

func main() {
	serverTerminated := make(chan interface{})
	stop := startMockServer(serverTerminated)

	logger := client.LoggerFunc(func(format string, args ...interface{}) {
		_, _ = fmt.Printf(format, args...)
	})

	cfg := client.NewConfig("key", "secret")
	cfg.BaseURL = "http://localhost:8080"
	cfg.Logger = logger

	cnt := client.New(cfg)

	req := merchantRequest{
		MerchantID: 1,
	}
	resp := &merchantDetails{}
	err := cnt.CallWithContext(context.Background(), "merchant.GetDetails", req, resp)
	if err != nil {
		fmt.Printf("request filed: %v\n", resp)
	} else {
		fmt.Printf("response: %v\n", *resp)
	}

	close(stop)
	<-serverTerminated
}

func startMockServer(terminated chan<- interface{}) chan<- interface{} {

	srv := &http.Server{Addr: ":8080"}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"jsonrpc": "2.0","result": {"merchant_id": 1, "name": "City Tours"},"id": "1"}`)
	})

	go func() {
		fmt.Printf("starting the server on %s...\n", srv.Addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("unable to start the server: %v", err)
		}
	}()

	stop := make(chan interface{}, 1)
	go func() {
		defer close(terminated)
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		fmt.Println("terminating the server...")
		if err := srv.Shutdown(ctx); err != nil {
			panic(fmt.Errorf("unable to gracefully terminate the server: %s", err))
		}

		fmt.Println("server is terminated")
	}()

	// wait for 1 second to let server start
	select {
	case <-time.After(1 * time.Second):
	}

	fmt.Println("server is running")
	return stop
}

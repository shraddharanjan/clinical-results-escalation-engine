package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type resultRequest struct {
	SourceSystem     string    `json:"source_system"`
	SourceResultID   string    `json:"source_result_id"`
	PatientReference string    `json:"patient_reference"`
	TestCode         string    `json:"test_code"`
	Value            float64   `json:"value"`
	Unit             string    `json:"unit"`
	ReportedAt       time.Time `json:"reported_at"`
}

type configuration struct {
	BaseURL      string
	RequestCount int
	Concurrency  int
	Timeout      time.Duration
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	config, err := loadConfiguration()
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: config.Timeout,
	}

	jobs := make(chan int)

	var (
		waitGroup    sync.WaitGroup
		latencyMutex sync.Mutex
		latencies    []time.Duration
		successCount int64
		failureCount int64
	)

	startedAt := time.Now()

	for workerNumber := 0; workerNumber < config.Concurrency; workerNumber++ {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for requestNumber := range jobs {
				latency, err := submitResult(
					client,
					config.BaseURL,
					requestNumber,
				)

				if err != nil {
					atomic.AddInt64(
						&failureCount,
						1,
					)
					continue
				}

				atomic.AddInt64(
					&successCount,
					1,
				)

				latencyMutex.Lock()
				latencies = append(
					latencies,
					latency,
				)
				latencyMutex.Unlock()
			}
		}()
	}

	for requestNumber := 0; requestNumber < config.RequestCount; requestNumber++ {
		jobs <- requestNumber
	}

	close(jobs)
	waitGroup.Wait()

	totalDuration := time.Since(startedAt)

	sort.Slice(
		latencies,
		func(left int, right int) bool {
			return latencies[left] <
				latencies[right]
		},
	)

	fmt.Println()
	fmt.Println("Clinical Results Load Test")
	fmt.Println("--------------------------")
	fmt.Printf(
		"Requests:       %d\n",
		config.RequestCount,
	)
	fmt.Printf(
		"Concurrency:    %d\n",
		config.Concurrency,
	)
	fmt.Printf(
		"Successful:     %d\n",
		successCount,
	)
	fmt.Printf(
		"Failed:         %d\n",
		failureCount,
	)
	fmt.Printf(
		"Total duration: %s\n",
		totalDuration,
	)

	if totalDuration > 0 {
		fmt.Printf(
			"Throughput:     %.2f requests/second\n",
			float64(successCount)/
				totalDuration.Seconds(),
		)
	}

	if len(latencies) > 0 {
		fmt.Printf(
			"p50 latency:    %s\n",
			percentile(latencies, 0.50),
		)
		fmt.Printf(
			"p95 latency:    %s\n",
			percentile(latencies, 0.95),
		)
		fmt.Printf(
			"p99 latency:    %s\n",
			percentile(latencies, 0.99),
		)
		fmt.Printf(
			"maximum:        %s\n",
			latencies[len(latencies)-1],
		)
	}

	return nil
}

func submitResult(
	client *http.Client,
	baseURL string,
	requestNumber int,
) (time.Duration, error) {
	severityIndex := requestNumber % 20

	value := 4.2

	switch {
	case severityIndex == 0:
		value = 6.8

	case severityIndex < 5:
		value = 5.8
	}

	requestBody := resultRequest{
		SourceSystem: "load-generator",
		SourceResultID: fmt.Sprintf(
			"LOAD-%d-%06d",
			time.Now().UnixNano(),
			requestNumber,
		),
		PatientReference: fmt.Sprintf(
			"P-LOAD-%06d",
			requestNumber,
		),
		TestCode: "serum_potassium",
		Value:    value,
		Unit:     "mmol/L",
		ReportedAt: time.Now().
			UTC().
			Add(-time.Minute),
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return 0, fmt.Errorf(
			"marshal request: %w",
			err,
		)
	}

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		baseURL+"/v1/results",
		bytes.NewReader(body),
	)
	if err != nil {
		return 0, fmt.Errorf(
			"create request: %w",
			err,
		)
	}

	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	startedAt := time.Now()

	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()

	_, _ = io.Copy(
		io.Discard,
		response.Body,
	)

	latency := time.Since(startedAt)

	if response.StatusCode != http.StatusCreated {
		return latency, errors.New(
			response.Status,
		)
	}

	return latency, nil
}

func percentile(
	values []time.Duration,
	percentileValue float64,
) time.Duration {
	if len(values) == 0 {
		return 0
	}

	index := int(
		percentileValue *
			float64(len(values)-1),
	)

	return values[index]
}

func loadConfiguration() (
	configuration,
	error,
) {
	baseURL := os.Getenv("LOAD_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	requestCount, err := integerEnvironment(
		"LOAD_REQUEST_COUNT",
		1000,
	)
	if err != nil {
		return configuration{}, err
	}

	concurrency, err := integerEnvironment(
		"LOAD_CONCURRENCY",
		50,
	)
	if err != nil {
		return configuration{}, err
	}

	timeout := 10 * time.Second

	if value := os.Getenv(
		"LOAD_REQUEST_TIMEOUT",
	); value != "" {
		timeout, err = time.ParseDuration(value)
		if err != nil {
			return configuration{}, fmt.Errorf(
				"parse LOAD_REQUEST_TIMEOUT: %w",
				err,
			)
		}
	}

	return configuration{
		BaseURL:      baseURL,
		RequestCount: requestCount,
		Concurrency:  concurrency,
		Timeout:      timeout,
	}, nil
}

func integerEnvironment(
	name string,
	defaultValue int,
) (int, error) {
	value := os.Getenv(name)

	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf(
			"parse %s: %w",
			name,
			err,
		)
	}

	if parsed <= 0 {
		return 0, fmt.Errorf(
			"%s must be greater than zero",
			name,
		)
	}

	return parsed, nil
}

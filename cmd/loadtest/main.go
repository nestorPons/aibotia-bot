// Comando loadtest: genera carga HTTP sobre la API para pruebas de rendimiento.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	var (
		base        = flag.String("base", "http://localhost:3000", "URL base de la API")
		key         = flag.String("key", "", "valor de x-api-key")
		paths       = flag.String("paths", "/health,/api/v1/users,/api/v1/instructions", "rutas separadas por coma")
		duration    = flag.Duration("duration", 20*time.Second, "duración de la prueba")
		concurrency = flag.Int("concurrency", 20, "número de workers concurrentes")
	)
	flag.Parse()

	routeList := splitPaths(*paths)
	if len(routeList) == 0 {
		fmt.Println("sin rutas que probar")
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var (
		total    atomic.Int64
		errors   atomic.Int64
		latMu    sync.Mutex
		latency  []time.Duration
		statuses sync.Map // int -> *int64
	)

	var wg sync.WaitGroup
	for w := 0; w < *concurrency; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			i := worker
			for ctx.Err() == nil {
				route := routeList[i%len(routeList)]
				i++
				start := time.Now()
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(*base, "/")+route, nil)
				if err != nil {
					continue
				}
				if *key != "" {
					req.Header.Set("x-api-key", *key)
				}
				resp, err := client.Do(req)
				elapsed := time.Since(start)
				if err != nil {
					// No contar las peticiones canceladas al finalizar la prueba.
					if ctx.Err() == nil {
						errors.Add(1)
					}
					continue
				}
				_ = resp.Body.Close()

				total.Add(1)
				if resp.StatusCode >= 400 {
					errors.Add(1)
				}
				counter, _ := statuses.LoadOrStore(resp.StatusCode, new(int64))
				atomic.AddInt64(counter.(*int64), 1)

				latMu.Lock()
				latency = append(latency, elapsed)
				latMu.Unlock()
			}
		}(w)
	}
	wg.Wait()

	report(total.Load(), errors.Load(), latency, &statuses, *duration)
}

func report(total, errs int64, latency []time.Duration, statuses *sync.Map, duration time.Duration) {
	fmt.Println("== Resultado de la prueba de carga ==")
	fmt.Printf("requests: %d   errores: %d   duración: %s\n", total, errs, duration)
	if duration > 0 {
		fmt.Printf("RPS: %.1f\n", float64(total)/duration.Seconds())
	}

	counts := map[int]int64{}
	statuses.Range(func(k, v any) bool {
		counts[k.(int)] = atomic.LoadInt64(v.(*int64))
		return true
	})
	fmt.Print("códigos de estado: ")
	codes := make([]int, 0, len(counts))
	for c := range counts {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	for _, c := range codes {
		fmt.Printf("%d=%d ", c, counts[c])
	}
	fmt.Println()

	if len(latency) == 0 {
		return
	}
	sort.Slice(latency, func(i, j int) bool { return latency[i] < latency[j] })
	fmt.Printf("latencia p50=%s p90=%s p99=%s max=%s\n",
		pct(latency, 0.50), pct(latency, 0.90), pct(latency, 0.99), latency[len(latency)-1])
}

func pct(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}

func splitPaths(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

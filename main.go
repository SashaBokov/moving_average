package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	symbols         []string      = []string{"BINANCE:BTCUSDT", "BINANCE:ETHUSDT", "BINANCE:ADAUSDT"}
	apiKey          string        = "ce4ecgaad3i3k9dfa8k0ce4ecgaad3i3k9dfa8kg"
	windowSize      int           = 60
	outputBuffer    int           = len(symbols) * windowSize
	storagePath     string        = "./storage"
	shutdownTimeout time.Duration = time.Second * 3
)

type Trade struct {
	Price         float64 `json:"p"`
	Symbol        string  `json:"s"`
	TimeMilliUNIX int64   `json:"t"`
}

type Input struct {
	Data []Trade `json:"data"`
	Type string  `json:"type"`
}

// Parse Using for parse Input and make average from similar symbols.
func (in Input) Parse() map[string]Trade {
	separatedTrades := make(map[string]Trade)

	for _, data := range in.Data {
		if average, consist := separatedTrades[data.Symbol]; consist {
			average.TimeMilliUNIX = (average.TimeMilliUNIX + data.TimeMilliUNIX) / 2
			average.Price = (average.Price + data.Price) / 2
			separatedTrades[data.Symbol] = average
			continue
		}

		separatedTrades[data.Symbol] = Trade{
			Symbol:        data.Symbol,
			TimeMilliUNIX: data.TimeMilliUNIX,
			Price:         data.Price,
		}
	}
	return separatedTrades
}

func init() {
	if os.Getenv("SYMBOLS") != "" {
		symbols = strings.Split(os.Getenv("SYMBOLS"), " ")
	}

	if os.Getenv("APIKEY") == "" {
		log.Panicln("apiKey is required")
	}

	apiKey = os.Getenv("APIKEY")

	if os.Getenv("storagePath") != "" {
		storagePath = os.Getenv("storagePath")
	}

	if os.Getenv("WINDOW_SIZE") != "" {
		windowSize, _ = strconv.Atoi(os.Getenv("WINDOW_SIZE"))
	}

	if os.Getenv("OUTPUT_BUFFER") != "" {
		outputBuffer, _ = strconv.Atoi(os.Getenv("OUTPUT_BUFFER"))
	}

	if os.Getenv("SHUTDOWN_TIMEOUT") != "" {
		duration, _ := strconv.Atoi(os.Getenv("SHUTDOWN_TIMEOUT"))
		shutdownTimeout = time.Duration(duration)
	}
}

func main() {
	w, resp, err := websocket.DefaultDialer.Dial(fmt.Sprintf("wss://ws.finnhub.io?token=%s", apiKey), nil)
	if err != nil {
		log.Panicln(err, resp)
	}
	defer w.Close()

	for _, s := range symbols {
		msg, _ := json.Marshal(map[string]interface{}{"type": "subscribe", "symbol": s})

		err = w.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			log.Panicln(err)
		}
	}

	symbolToWriter := make(map[string]io.Writer)
	for _, symbol := range symbols {
		f, err := os.Create(path.Join(storagePath, symbol+".json.out"))
		if err != nil {
			log.Panicln(err)
		}

		symbolToWriter[symbol] = f
	}

	ctx, cancelFn := context.WithTimeout(context.Background(), shutdownTimeout)

	symbolToWorkerInput := make(map[string]chan Trade)
	workersOutput := make(chan Trade, outputBuffer)

	for _, pair := range symbols {
		symbolToWorkerInput[pair] = make(chan Trade, windowSize)
		go Worker(ctx, windowSize, symbolToWorkerInput[pair], workersOutput)
	}

	go Listen(ctx, w, symbolToWorkerInput)
	go Storage(ctx, workersOutput, symbolToWriter)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit
	cancelFn()
	w.Close()
}

// Listen using for listen websocket.Conn and write workers inputs.
func Listen(ctx context.Context, w *websocket.Conn, pairToWorkerInput map[string]chan Trade) {
	defer func() {
		for _, ch := range pairToWorkerInput {
			close(ch)
		}
	}()

	var input Input

	for {
		select {
		case <-ctx.Done():
			return

		default:
			err := w.ReadJSON(&input)
			if err != nil {
				log.Panicln(err)
			}

			trades := input.Parse()
			for pair, trade := range trades {
				//log.Printf("-----------------------------%s : %f : %s : %d--------------------------", pair, trade.Price, time.UnixMilli(trade.TimeMilliUNIX).String(), len(trades))
				pairToWorkerInput[pair] <- trade
			}
		}

	}
}

// Worker need for make moving average.
func Worker(ctx context.Context, windowSize int, in <-chan Trade, out chan<- Trade) {
	defer close(out)

	window := make([]Trade, 0, windowSize)
	average := Trade{}

	for i := 0; i < windowSize; i++ {
		window = append(window, <-in)
		average.Price += window[i].Price
	}

	average.TimeMilliUNIX = window[windowSize-1].TimeMilliUNIX
	average.Price /= float64(windowSize)
	average.Symbol = window[windowSize-1].Symbol

	cursor := 0

	for newValue := range in {
		out <- average

		average.Price += (window[cursor].Price - newValue.Price) / float64(windowSize)
		average.TimeMilliUNIX = newValue.TimeMilliUNIX

		window[cursor] = newValue

		if cursor == windowSize-1 {
			cursor = 0
		}

		cursor++
	}
}

func Storage(ctx context.Context, in <-chan Trade, symbolToWriter map[string]io.Writer) {
	encoders := make(map[string]*json.Encoder)
	for pair, w := range symbolToWriter {
		encoders[pair] = json.NewEncoder(w)
	}

	for average := range in {
		encoders[average.Symbol].Encode(average)
	}
}

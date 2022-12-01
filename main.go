package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"runtime/trace"
	"time"
)

type Input struct {
	Data []Trade `json:"data"`
	Type string  `json:"type"`
}

type Trade struct {
	Price         float64 `json:"p"`
	Symbol        string  `json:"s"`
	TimeMilliUNIX int64   `json:"t"`
}

func main() {
	apiKey := ""

	w, resp, err := websocket.DefaultDialer.Dial(fmt.Sprintf("wss://ws.finnhub.io?token=%s", apiKey), nil)
	if err != nil {
		log.Panicln(err, resp)
	}
	defer w.Close()

	symbols := []string{"BINANCE:BTCUSDT"} // , "BINANCE:ETHUSDT", "BINANCE:ADAUSDT"}
	for _, s := range symbols {
		msg, _ := json.Marshal(map[string]interface{}{"type": "subscribe", "symbol": s})

		err = w.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			panic(err)
		}
	}

	input := make(chan Trade)
	output := make(chan Trade)

	go Worker(60, input, output)

	var msg Input

	for {
		time.Sleep(time.Millisecond * 500)
		select {
		case result := <-output:
			log.Printf("--------------------------WORKER---%s : %f : %s---WORKER-----------------------", result.Symbol, result.Price, time.UnixMilli(result.TimeMilliUNIX).String())

		default:
			err := w.ReadJSON(&msg)
			if err != nil {
				panic(err)
			}

			//log.Printf("-----------------------------%s : %f : %s--------------------------", msg.Type, msg.Data[0].Price, time.UnixMilli(msg.Data[0].TimeMilliUNIX).String())
			//log.Printf("Message from server: %+v\n", msg.Data)

			av := msg.Parse()
			for key, val := range av {
				log.Printf("-----------------------------%s : %f : %s : %d--------------------------", key, val.Price, time.UnixMilli(val.TimeMilliUNIX).String(), len(av))
			}

			if average, consist := av["BINANCE:BTCUSDT"]; consist {
				input <- average
				log.Printf("\n-----------------------------%s : %f : %s--------------------------\n", average.Symbol, average.Price, time.UnixMilli(average.TimeMilliUNIX).String())
			}
		}
	}

	//_, message, err := w.ReadMessage()
	//if err != nil {
	//	panic(err)
	//}
	//fmt.Printf("%s", message)
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

// Worker need for make moving average.
func Worker(windowSize int, in <-chan Trade, out chan<- Trade) {
	trace.NewTask(context.Background(), "WORKER")
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

// 15 + 10 + 45 + 100 + 30 = 200 / 5 = 40 old
// 20 + 10 + 45 + 100 + 30 = 205 / 5 = 41 new
// delta = 20 - 15
// windowDelta = delta / windowSize =  5 / 5 = 1

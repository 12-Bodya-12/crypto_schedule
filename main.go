package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/eiannone/keyboard"
	"github.com/gosuri/uilive"
	"github.com/guptarohit/asciigraph"
)

var (
	symbols = []string{"BTC_USD", "LTC_USD", "ETH_USD"}
	apiUrls = map[string]string{
		"BTC_USD": "https://api.binance.com/api/v3/ticker/price?symbol=BTCUSDT",
		"LTC_USD": "https://api.binance.com/api/v3/ticker/price?symbol=LTCUSDT",
		"ETH_USD": "https://api.binance.com/api/v3/ticker/price?symbol=ETHUSDT",
	}
)

// Linux
// func clearScreen() {
// 	cmd := exec.Command("clear") // Используем команду clear для очистки консоли
// 	cmd.Stdout = os.Stdout
// 	cmd.Run()
// }

// windows
func clearScreen() {
	cmd := exec.Command("cmd", "/C", "cls") // Для Windows используем команду cmd /C cls
	cmd.Stdout = os.Stdout
	cmd.Run()
}

type APIResponse struct {
	Price string `json:"price"`
}

func displayMenu() {
	fmt.Println("Меню:")
	for i, symbol := range symbols {
		fmt.Printf("%d. %s\n", i+1, symbol)
	}
	fmt.Println("Нажмите 1-3 для выбора графика, нажмите BACKSPACE чтобы вернуться в меню, нажмите q для выхода.")
}

func fetchPrice(symbol string) (float64, error) {
	resp, err := http.Get(apiUrls[symbol])
	if err != nil {
		return 0, fmt.Errorf("error fetching price for %s: %v", symbol, err)
	}
	defer resp.Body.Close()

	var result APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("error decoding response: %v", err)
	}

	// Преобразовываем цену из строки в плавающую
	price, err := strconv.ParseFloat(result.Price, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing price for %s: %v", symbol, err)
	}
	return price, nil
}

func worker(symbol string, prices *[]float64, mutex *sync.Mutex, stop chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-stop:
			return
		default:
			price, err := fetchPrice(symbol)
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}
			mutex.Lock()
			*prices = append(*prices, price)
			if len(*prices) > 100 {
				*prices = (*prices)[1:]
			}
			mutex.Unlock()
			time.Sleep(1 * time.Second)
		}
	}
}

func printGraph(writer *uilive.Writer, symbol string, prices *[]float64, mutex *sync.Mutex, stop chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-stop:
			return
		default:

			mutex.Lock()
			if len(*prices) > 0 {
				// Создаем график
				graph := asciigraph.Plot(*prices, asciigraph.Width(50), asciigraph.Height(10), asciigraph.Caption(symbol))

				// Получаем текущее время и дату
				currentTime := time.Now().Format("15:04:05")
				currentDate := time.Now().Format("2006-01-02")

				// Получаем последнюю цену из среза
				lastPrice := (*prices)[len(*prices)-1]

				// Выводим график, символ, последнюю цену, текущее время и текущую дату.
				fmt.Fprintf(writer, "\n%s\nЦена %s: %.2f\n\nТекущее время: %s\nТекущая дата: %s\n", graph, symbol, lastPrice, currentTime, currentDate)
			}
			mutex.Unlock()

			writer.Flush()
			time.Sleep(1 * time.Second)
		}
	}
}

func main() {
	// Открытие клавиатуры для ввода
	if err := keyboard.Open(); err != nil {
		log.Fatal(err)
	}
	defer keyboard.Close()

	var (
		currentSymbol = "BTC_USD"
		prices        []float64
		mutex         = &sync.Mutex{}
		stopWorker    chan struct{}
		stopGraph     chan struct{}
		graphMode     = false
		wg            sync.WaitGroup
		writer        = uilive.New()
		stopFlag      bool // Флаг для контроля закрытия каналов
	)

	displayMenu()

	for {
		if !graphMode {
			fmt.Print("\nВведите команду: ")
		}

		// Считываем символ с клавиатуры
		key, _, err := keyboard.GetKey()
		if err != nil {
			log.Fatalf("Ошибка чтения ввода: %v", err)
		}

		// Обрабатываем нажатие на клавишу Backspace
		if key == 0 && graphMode {
			// Если мы в режиме графика, выходим из него
			graphMode = false
			if stopFlag {
				close(stopWorker) // Закрываем каналы только один раз
				close(stopGraph)
				wg.Wait()
				stopFlag = false
			}
			displayMenu() // Возвращаем меню
			continue
		}

		// Преобразуем символ в строку
		command := string(key)

		if !graphMode {
			switch command {
			case "1", "2", "3":
				// Закрываем старые потоки перед запуском новых
				if stopFlag {
					close(stopWorker)
					close(stopGraph)
					wg.Wait()
				}
				stopWorker = make(chan struct{})
				stopGraph = make(chan struct{})
				currentSymbol = map[string]string{
					"1": "BTC_USD",
					"2": "LTC_USD",
					"3": "ETH_USD",
				}[command]

				prices = []float64{}
				wg.Add(2)
				go func() {
					worker(currentSymbol, &prices, mutex, stopWorker, &wg)
				}()
				go func() {
					clearScreen()
					printGraph(writer, currentSymbol, &prices, mutex, stopGraph, &wg)
				}()
				graphMode = true
				stopFlag = true // Устанавливаем флаг
			case "q":
				// Завершаем работу
				if stopFlag {
					close(stopWorker)
					close(stopGraph)
					wg.Wait()
				}
				fmt.Println("\nВыходим...")
				return
			}
		} else {
			switch command {
			case "q":
				// Завершаем работу
				if stopFlag {
					close(stopWorker)
					close(stopGraph)
					wg.Wait()
				}
				fmt.Println("\nВыходим...")
				return
			}
		}
	}
}

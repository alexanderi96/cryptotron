package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/csv"
	"fmt"
	"image/png"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"

	"github.com/NicoNex/echotron/v3"
	"github.com/go-yaml/yaml"

	openai "github.com/sashabaranov/go-openai"
)

type bot struct {
	chatID int64
	echotron.API
}

var (
	//go:embed telegram_token
	telegram_token string

	//go:embed openai_api_key
	openai_api_key string

	//go:embed admin
	admin string

	dsp *echotron.Dispatcher

	conf_file_path string = ".config/wallet-tracker/config.yaml"
	work_dir       string = "Documents/wallet-tracker/wallet_"
	csv_file       string

	config map[string]string
	client *openai.Client
)

func init() {
	if len(telegram_token) == 0 {
		log.Fatal("Empty telegram_token file")
	}

	if len(admin) == 0 {
		log.Fatal("Empty admin file")
	}

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	homeDir := usr.HomeDir

	conf_file_path = fmt.Sprintf("%s/%s", homeDir, conf_file_path)
	if !checkPathExists(conf_file_path) {
		log.Fatal("Config file not found: ", conf_file_path)
	}

	//read wallet tracker config file from ~/.config/wallet-tracker/config.yaml and put the variables in the config map
	config, err = readConfig(conf_file_path)
	if err != nil {
		log.Fatalf(fmt.Sprintf("Unable to read the config file: %s", err))
	}
	log.Println("Config file loaded")

	work_dir = fmt.Sprintf("%s/%s%s", homeDir, work_dir, config["wallet_address"])
	if !checkPathExists(work_dir) {
		log.Fatal("Work folder not found")
	}
	log.Println("Work folder loaded")
	csv_file = fmt.Sprintf("%s/%s_data.csv", work_dir, config["token"])

	client = openai.NewClient(openai_api_key)
}

func newBot(chatID int64) echotron.Bot {
	bot := &bot{
		chatID,
		echotron.NewAPI(telegram_token),
	}
	go bot.selfDestruct(time.After(time.Hour))
	return bot
}

func (b *bot) selfDestruct(timech <-chan time.Time) {
	<-timech
	b.SendMessage("goodbye", b.chatID, nil)
	dsp.DelSession(b.chatID)
}

// Returns the message from the given update.
func message(update *echotron.Update) string {
	if update == nil {
		return ""
	} else if update.Message != nil {
		return update.Message.Text
	} else if update.EditedMessage != nil {
		return update.EditedMessage.Text
	} else if update.CallbackQuery != nil {
		return update.CallbackQuery.Data
	}
	return ""
}

func (b *bot) Update(update *echotron.Update) {
	log.Println("Message recieved from: " + strconv.FormatInt(b.chatID, 10))

	if strconv.Itoa(int(b.chatID)) != admin {
		b.SendMessage("📷", b.chatID, nil)
	} else {

		switch msg := message(update); {
		case strings.HasPrefix(msg, "/ping"):
			b.SendMessage("pong", b.chatID, nil)

		case strings.HasPrefix(msg, "/graph"):
			msg_split := strings.Split(msg, " ")
			if len(msg_split) != 2 {
				b.SendMessage("Invalid command", b.chatID, nil)
				return
			}

			now := time.Now()
			endDate := now.Format("02-01-2006 15:04:05")
			startDate := ""

			switch msg_split[1] {
			case "24h":
				startDate = now.Add(-24 * time.Hour).Format("02-01-2006 15:04:05")

			case "7d":
				startDate = now.AddDate(0, 0, -7).Format("02-01-2006 15:04:05")

			case "30d":
				startDate = now.AddDate(0, 0, -30).Format("02-01-2006 15:04:05")

			case "90d":
				startDate = now.AddDate(0, 0, -90).Format("02-01-2006 15:04:05")

			case "365d":
				startDate = now.AddDate(0, 0, -365).Format("02-01-2006 15:04:05")

			case "all":
			default:
				b.SendMessage("Invalid command, sending all.", b.chatID, nil)
			}

			b.SendDocument(echotron.NewInputFileBytes( msg_split[1] + "_graph.png", generateGraph(config, csv_file, startDate, endDate)), b.chatID, nil)

		case strings.HasPrefix(msg, "/data"):
			if checkPathExists(csv_file) {
				b.SendMessage("Sending the latest data", b.chatID, nil)
				b.SendDocument(echotron.NewInputFilePath(csv_file), b.chatID, nil)
			} else {
				b.SendMessage("Data not found", b.chatID, nil)
			}

		case strings.HasPrefix(msg, "/gpt"):
			msg_split := strings.Split(msg, " ")
			
			prompt := ""
			data_context_prompt := "\n\nIl prezzo è espresso in " + config["currency"] + ", mentre la quantità posseduta personalmente e quella del burn wallet sono espresse in " + config["token"] + ". Il numero massimo di token disponibili è : " + config["total_supply"] + ".\nPuoi arrotondare i dati alle ultime 3 cifre significative, e mostrarmi una percentuale di aumento o diminuzione rispetto all'inizio e la fine del periodo di riferimento.\n\n"
			rows_to_analyze := 0
			msg_to_send := ""
			var err error

			if len(msg_split) <= 2 {
				b.SendMessage("Invalid command", b.chatID, nil)
				return
			} else if len(msg_split) > 2 {
				rows_to_analyze, err = strconv.Atoi(msg_split[len(msg_split)-1])
				if err != nil {
					prompt = strings.Join(msg_split[1:len(msg_split)], " ")
				} else {
					prompt = strings.Join(msg_split[1:len(msg_split)-1], " ")
				}
			}

			if rows_to_analyze > 0 {
				headers, err := ReadCsvHeaders(csv_file)
				if err != nil {
					b.SendMessage("Data not found", b.chatID, nil)
					return
				} else {
					rows, err := ReadLastNCsvRows(csv_file, rows_to_analyze)
					if err != nil {
						b.SendMessage("Header not found", b.chatID, nil)
						return
					} 
					msg_to_send = prompt + data_context_prompt + headers + rows
				}
			} else {
				msg_to_send = prompt
			}
			

			b.SendMessage("Analyzing message:\n\n" + msg_to_send, b.chatID, nil)
			msg, err := SendMessageToChatGPT(msg_to_send, "gpt-3.5-turbo")
			if err != nil {
				b.SendMessage(err.Error(), b.chatID, nil)
			} else {
				b.SendMessage(msg, b.chatID, nil)
			}

		default:
			b.SendMessage("👀", b.chatID, nil)
		}
	}
}

// the readConfig function, it reads the yaml file and puts the variables in the config map
func readConfig(path string) (map[string]string, error) {

	config := make(map[string]string)
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// read the yaml file
	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func createFolderIfNotExists(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.Mkdir(path, 0755)
		log.Println("Directory '" + path + "' created")
	}
}

func checkPathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func generateGraph(config map[string]string, csvFile, startDate, endDate string) []byte {
	log.Printf("Creazione dei grafici dal file: '%s'.", csvFile)

	file, err := ioutil.ReadFile(csvFile)
	if err != nil {
		log.Fatal(err)
	}

	reader := csv.NewReader(bytes.NewReader(file))
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	// Parse and filter the records
	var dates []time.Time
	var values [][]float64
	for i := 1; i < len(records[0]); i++ {
		values = append(values, []float64{})
	}

	for _, record := range records[1:] {
		date, _ := time.Parse("02-01-2006 15:04:05", record[0])

		if startDate != "" {
			start, _ := time.Parse("02-01-2006 15:04:05", startDate)
			if date.Before(start) || date.Equal(start) { // Change this line
				continue
			}
		}

		if endDate != "" {
			end, _ := time.Parse("02-01-2006 15:04:05", endDate)
			if date.After(end) || date.Equal(end) { // Change this line
				continue
			}
		}

		dates = append(dates, date)

		for i := 1; i < len(record); i++ {
			value, _ := strconv.ParseFloat(record[i], 64)
			values[i-1] = append(values[i-1], value)
		}
	}

	// Create the combined image
	img := vgimg.New(6*vg.Inch, 4*vg.Inch*vg.Length(len(records[0])-1))
	dc := draw.New(img)

	for i, label := range records[0][1:] {
		// Create the plot
		p := plot.New()
		//p.Title.Text = fmt.Sprintf("%s\ntelegram_Token: %s (%s)\nContratto: %s\nWallet: %s", label, telegram_token, config["telegram_token"], config["contract"], config["wallet_address"])
		p.Title.Text = fmt.Sprintf("%s", label)
		p.X.Label.Text = "Data"
		p.Y.Label.Text = "Valore"

		pts := make(plotter.XYs, len(dates))
		for j := range dates {
			pts[j].X = float64(dates[j].Unix())
			pts[j].Y = values[i][j]
		}

		line, err := plotter.NewLine(pts)
		if err != nil {
			log.Fatal(err)
		}
		p.Add(line)
		p.X.Tick.Marker = plot.TimeTicks{Format: "02-01-06 15:04", Ticker: plot.DefaultTicks{}}

		// Draw the plot on the combined image
		c := draw.Canvas{
			Canvas: draw.Crop(dc, 0, 4*vg.Inch*vg.Length(i), 6*vg.Inch, 4*vg.Inch),
			Rectangle: vg.Rectangle{
				Min: vg.Point{X: 0, Y: 4 * vg.Inch * vg.Length(i)},
				Max: vg.Point{X: 6 * vg.Inch, Y: 4 * vg.Inch * (vg.Length(i) + 1)},
			},
		}
		p.Draw(c)
	}

	// Save the combined image to a bytes.Buffer
	var buf bytes.Buffer
	png.Encode(&buf, img.Image())

	return buf.Bytes()
}

func SendMessageToChatGPT(message string, engineID string) (string, error) {
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: message,
				},
			},
		},
	)

	if err != nil {
		log.Print("ChatCompletion error: %v\n", err)
		return "", err
	}

	return resp.Choices[0].Message.Content, nil
}

func ReadCsvHeaders(filename string) (string, error) {
	// Apri il file CSV in modalità lettura
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Leggi le intestazioni del file CSV
	reader := csv.NewReader(file)
	headers, err := reader.Read()
	if err != nil {
		return "", err
	}

	// Concatena le intestazioni in una stringa
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	writer.Write(headers)
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return buffer.String(), nil
}


func ReadLastNCsvRows(filename string, n int) (string, error) {
	// Apri il file CSV in modalità lettura
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Leggi tutte le righe del file CSV
	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return "", err
	}

	// Calcola l'indice della prima riga da restituire
	start := len(rows) - n
	if start < 0 {
		start = 0
	}

	// Concatena le ultime n righe del file CSV in una stringa
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	writer.WriteAll(rows[start:])
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func main() {
	dsp := echotron.NewDispatcher(telegram_token, newBot)
	log.Println("Running Cryptotron Bot...")

	for {
		log.Println(dsp.Poll())
		log.Println("Lost connection, waiting one minute...")
		// In case of connection issues wait 1 minute before trying to reconnect.
		time.Sleep(1 * time.Minute)
	}
}

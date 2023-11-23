package main

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"fmt"
	"image/png"
	"log"
	"os"
	"os/user"
	"regexp"
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
)

type bot struct {
	chatID int64
	echotron.API
}

var (
	//go:embed telegram_token
	telegramToken string

	//go:embed admin
	admin string

	//dsp *echotron.Dispatcher

	confFilePath string = ".config/wallet-tracker/config.yaml"
	workDir      string = "Documents/wallet-tracker/wallet_"
	csvFile      string

	config map[string]string

	commands = []echotron.BotCommand{
		{Command: "/ping", Description: "check bot status"},
		{Command: "/graph", Description: "send the graph for a given time range"},
		{Command: "/data", Description: "sends the latest data"},
	}

	// parseMarkdown = &echotron.MessageOptions{ParseMode: echotron.MarkdownV2}
)

func init() {
	if len(telegramToken) == 0 {
		log.Fatal("Empty telegramToken file")
	}

	if len(admin) == 0 {
		log.Fatal("Empty admin file")
	}

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	homeDir := usr.HomeDir

	confFilePath = fmt.Sprintf("%s/%s", homeDir, confFilePath)
	if !checkPathExists(confFilePath) {
		log.Fatal("Config file not found: ", confFilePath)
	}

	//read wallet tracker config file from ~/.config/wallet-tracker/config.yaml and put the variables in the config map
	config, err = readConfig(confFilePath)
	if err != nil {
		log.Fatalf(fmt.Sprintf("Unable to read the config file: %s", err))
	}
	log.Println("Config file loaded")

	workDir = fmt.Sprintf("%s/%s%s", homeDir, workDir, config["wallet_address"])
	if !checkPathExists(workDir) {
		log.Fatal("Work folder not found")
	}
	log.Println("Work folder loaded")
	csvFile = fmt.Sprintf("%s/%s_data.csv", workDir, config["token"])

	go setCommands()
}

func setCommands() {
	api := echotron.NewAPI(telegramToken)
	api.SetMyCommands(nil, commands...)
}

func newBot(chatID int64) echotron.Bot {
	bot := &bot{
		chatID,
		echotron.NewAPI(telegramToken),
	}
	//go bot.selfDestruct(time.After(time.Hour))
	return bot
}

// func (b *bot) selfDestruct(timech <-chan time.Time) {
// 	<-timech
// 	b.SendMessage("goodbye", b.chatID, nil)
// 	dsp.DelSession(b.chatID)
// }

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
		b.SendMessage("👀", b.chatID, nil)
	} else {

		switch msg := message(update); {
		case strings.HasPrefix(msg, "/ping"):
			b.SendMessage("pong", b.chatID, nil)

		case strings.HasPrefix(msg, "/graph"):
			msgSplit := strings.Split(msg, " ")
			if len(msgSplit) == 1 {
				startDate, endDate := calculateTimeRange()
				b.SendDocument(echotron.NewInputFileBytes("24h_graph.png", generateGraph(config, csvFile, startDate, endDate)), b.chatID, nil)
				return
			} else if len(msgSplit) != 2 {
				b.SendMessage("Invalid command. Usage: /graph <time range>, where <time range> is a number followed by 'h' or 'd' (e.g. '24h' or '7d'). Sending graph for last 24 hours.", b.chatID, nil)
				startDate, endDate := calculateTimeRange()
				b.SendDocument(echotron.NewInputFileBytes("24h_graph.png", generateGraph(config, csvFile, startDate, endDate)), b.chatID, nil)
				return
			}

			startDate, endDate := calculateTimeRange(msgSplit[1])
			name := msgSplit[1]

			if startDate == "" {
				b.SendMessage("Invalid time range. Usage: /graph <time range>, where <time range> is a number followed by 'h' or 'd' (e.g. '24h' or '7d'). Sending graph for last 24 hours.", b.chatID, nil)
				startDate, endDate = calculateTimeRange()
				name = "default"
			}

			b.SendDocument(echotron.NewInputFileBytes(name+"_graph.png", generateGraph(config, csvFile, startDate, endDate)), b.chatID, nil)

		case strings.HasPrefix(msg, "/data"):
			if checkPathExists(csvFile) {
				b.SendMessage("Sending the latest data", b.chatID, nil)
				b.SendDocument(echotron.NewInputFilePath(csvFile), b.chatID, nil)
			} else {
				b.SendMessage("Data not found", b.chatID, nil)
			}

		default:
			b.SendMessage("📈", b.chatID, nil)
		}
	}
}

func calculateTimeRange(timeRange ...string) (string, string) {
	now := time.Now()
	endDate := now.Format("02-01-2006 15:04:05")
	startDate := ""

	if len(timeRange) > 0 {
		// Extract numerical and unit components from time range string
		re := regexp.MustCompile(`^(\d+)([dh])$`)
		matches := re.FindStringSubmatch(timeRange[0])

		if len(matches) == 3 {
			num, err := strconv.Atoi(matches[1])
			if err != nil {
				num = 24
			}

			unit := matches[2]

			// Calculate start date based on numerical and unit components
			switch unit {
			case "h":
				startDate = now.Add(-time.Duration(num) * time.Hour).Format("02-01-2006 15:04:05")

			case "d":
				startDate = now.AddDate(0, 0, -num).Format("02-01-2006 15:04:05")
			}
		} else {
			timeRange[0] = "24h"
		}
	} else {
		startDate = now.Add(-24 * time.Hour).Format("02-01-2006 15:04:05")
	}

	return startDate, endDate
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

// func createFolderIfNotExists(path string) {
// 	if _, err := os.Stat(path); os.IsNotExist(err) {
// 		os.Mkdir(path, 0755)
// 		log.Println("Directory '" + path + "' created")
// 	}
// }

func checkPathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func generateGraph(config map[string]string, csvFile, startDate, endDate string) []byte {
	log.Printf("Creazione dei grafici dal file: '%s'.", csvFile)

	file, err := os.ReadFile(csvFile)
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
		//p.Title.Text = fmt.Sprintf("%s\ntelegramToken: %s (%s)\nContratto: %s\nWallet: %s", label, telegramToken, config["telegramToken"], config["contract"], config["wallet_address"])
		p.Title.Text = label
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
	dsp := echotron.NewDispatcher(telegramToken, newBot)
	log.Println("Running Cryptotron Bot...")

	for {
		log.Println(dsp.Poll())
		log.Println("Lost connection, waiting one minute...")
		// In case of connection issues wait 1 minute before trying to reconnect.
		time.Sleep(1 * time.Minute)
	}
}

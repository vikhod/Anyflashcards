package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/burke/nanomemo/supermemo"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func downloadFile(URL, filePath string) error {

	//Get the response bytes from the url
	response, err := http.Get(URL)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {

		// Set up timeout
		updateConfig := tgbotapi.NewUpdate(0)
		updateConfig.Timeout = 60

		return fmt.Errorf("received non 200 response code")
	}

	//Create a empty file
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	//Write the bytes to the fiel
	_, err = io.Copy(file, response.Body)
	if err != nil {
		return err
	}

	return nil
}

func readDictionaryFromDisc(csvPath string) (dictionary Dictionary) {

	dictionary.ID = primitive.NewObjectID()
	dictionary.FilePath = csvPath
	dictionary.FactSet, _ = readFactsFromDisc(csvPath)

	return dictionary
}

func readFactsFromDisc(csvPath string) (factSet FactSet, err error) {

	smFactSet, err := loadAllFacts(csvPath)
	if err != nil {
		return nil, err
	}
	factSet = convertToFactSet(&smFactSet)

	return factSet, nil
}

func writeFactsToDisc(csvPath string, factSet FactSet) error {

	file, err := os.OpenFile(csvPath, os.O_WRONLY, 0660)
	if err != nil {
		return err
	}

	csvw := csv.NewWriter(file)

	for _, fact := range factSet {
		ef := fmt.Sprintf("%0.6f", fact.FactMetadata.Ef)
		n := fmt.Sprintf("%d", fact.FactMetadata.N)
		interval := fmt.Sprintf("%d", fact.FactMetadata.Interval)

		csvw.Write([]string{fact.Question, fact.Answer, ef, n, interval, fact.FactMetadata.IntervalFrom})
	}

	csvw.Flush()

	if err = file.Close(); err != nil {
		return err
	}

	return nil
}

func loadAllFacts(csvPath string) (smFactSet supermemo.FactSet, err error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	csvr := csv.NewReader(f)
	csvr.FieldsPerRecord = -1
	for {
		record, err := csvr.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		smFactSet, err = addFact(smFactSet, record)
		if err != nil {
			return nil, err
		}
	}

	return smFactSet, nil
}

func addFact(fs supermemo.FactSet, record []string) (supermemo.FactSet, error) {
	var fact *supermemo.Fact
	switch len(record) {
	case 2:
		fact = supermemo.NewFact(record[0], record[1])

	case 6:
		q := record[0]
		a := record[1]
		ef, err := strconv.ParseFloat(record[2], 64)
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(record[3], 10, 64)
		if err != nil {
			return nil, err
		}
		interval, err := strconv.ParseInt(record[4], 10, 64)
		if err != nil {
			return nil, err
		}
		intervalFrom := record[5]
		fact, err = supermemo.LoadFact(q, a, ef, int(n), int(interval), intervalFrom)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("invalid record format")
	}

	fs = append(fs, fact)

	return fs, nil
}

func pushDictionary(bot *tgbotapi.BotAPI, update *tgbotapi.Update) error {
	if update.Message.Document != nil {
		if update.Message.Document.MimeType == "text/csv" || update.Message.Document.MimeType == "text/comma-separated-values" {

			fileDirectUrl, err := bot.GetFileDirectURL(update.Message.Document.FileID)
			if err != nil {
				return err
			}

			// Make dir and download file
			if err := os.Mkdir(strconv.Itoa(update.Message.From.ID), os.ModePerm); err != nil {
				return err
			}
			csvDictionaryPath := "./" + strconv.Itoa(update.Message.From.ID) + "/" + update.Message.Document.FileName

			if err := downloadFile(fileDirectUrl, csvDictionaryPath); err != nil {
				return err
			}

			// Push dict to postgress
			factSet, err := readFactsFromDisc(csvDictionaryPath)
			if err != nil {
				return err
			}
			if err := dumpFactsToBase(update.Message.From, &factSet); err != nil {
				return err
			}

			// Reset waiting bool
			waitingForDictionary = false

			showMessage(bot, update.Message.From.ID, "Vocabulary pushed.")
			showMainMeny(bot, update.Message.From.ID)

		} else {
			// Pushed not .csv file
			showMessage(bot, update.Message.From.ID, "Your file is not .csv. Sent please .csv file.")
		}
	} else {
		// Pushed something but not file
		showMessage(bot, update.Message.From.ID, "Still waiting for your own dictionary .csv file.")
	}

	return nil
}

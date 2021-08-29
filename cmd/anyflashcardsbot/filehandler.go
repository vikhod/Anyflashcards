package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
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

type Dictionary struct {
	ID            primitive.ObjectID `bson:"_id"`
	FilePath      string             `bson:"filePath"`
	FactSet       supermemo.FactSet  `bson:"factSet"`
	OwnerUsername string             `bson:"ownerUsername"`
	OwnerID       int                `bson:"ownerId"`
}

func loadDictionary(csvPath string) (dictionary Dictionary) {

	dictionary.ID = primitive.NewObjectID()
	dictionary.FilePath = csvPath
	dictionary.FactSet = loadAllFacts(csvPath)

	return dictionary
}

func loadAllFacts(csvPath string) supermemo.FactSet {
	f, err := os.Open(csvPath)
	if err != nil {
		log.Fatalf("Couldn't open %s: %s\n", csvPath, err.Error())
	}
	defer f.Close()

	var fs supermemo.FactSet

	csvr := csv.NewReader(f)
	csvr.FieldsPerRecord = -1
	for {
		record, err := csvr.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("Couldnt' read csv: %s\n", err.Error())
		}
		fs, err = addFact(fs, record)
		if err != nil {
			log.Fatalf("Couldnt' parse csv: %s\n", err.Error())
		}
	}

	return fs
}

func addFact(fs supermemo.FactSet, record []string) (supermemo.FactSet, error) {
	var fact *supermemo.Fact
	switch len(record) {
	case 2:
		fact = supermemo.NewFact(record[0], record[1])
		/*
			q := record[0]
			a := record[1]
			ef := 2.5
			n := 0
			interval := 0
			intervalFrom := "0001-01-01"
			var err error
			fact, err = supermemo.LoadFact(q, a, ef, int(n), int(interval), intervalFrom)
			if err != nil {
				return nil, err
			}
		*/
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

package main

import (
	"context"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/burke/nanomemo/supermemo"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"go.mongodb.org/mongo-driver/bson"
)

var help = `
I can help you to remember a lot of the new words.
Lern words with your own dictionary.
	
You can control me by sending these commands:

/run - Start learning your dictionary
/random20 - Start learning 10 random words from your dictionary
/settings - Configure bot parameters
/pushdict - Push your own dictionary
/pulldict - Pull your own dictionary
/deldict - Delete your own dictionary
/chdict - Chouse dictionary for learning
`

var mainMenuKeyboard = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Quiz", "quiz"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Hot20", "Hot20"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Settings", "settings"),
	),
)

var settingsKeyboard = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Push dictionary", "pushVocab"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Pull dictionary", "pullVocab"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Delete dictionary", "delVocab"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Chouse dictionary", "chVocab"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("<< Back", "back"),
	),
)

var libraryForReview = map[int]supermemo.FactSet{}
var countForReview = map[int]int{}
var membership = map[int]string{}

type Stopwatch struct {
	start time.Time
	mark  time.Duration
}

var stopwatch = map[int]Stopwatch{}
var quality = map[int]int{}

var defaultLibraryDirPath = "./configs/dictionaries"

func main() {
	// Create bot
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TOKEN"))
	if err != nil {
		log.Panic(err)
	}

	// More information about the requests being sent to Telegram
	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Set up timeout
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60

	// Get updates from bot
	updates, err := bot.GetUpdatesChan(updateConfig)
	if err != nil {
		log.Panic(err)
	}

	// Optional: wait for updates and clear them if you don't want to handle
	// a large backlog of old messages
	time.Sleep(time.Millisecond * 500)
	updates.Clear()

	// Set waiting bool
	waitingForPushVocab := false

	// Connect to database
	if err := connectMongoDb(); err != nil {
		log.Panic(err)
	}

	// Create and fill default library in database
	if err := updateDefaultLibrary(defaultLibraryDirPath, &bot.Self); err != nil {
		log.Panic(err)
	}

	// Fill users map for security checking
	if err := fillMembershipMap(); err != nil {
		log.Panic(err)
	}

	// Add anyflashcardsbot user to database
	addNewUser(bot, &bot.Self)

	// Go through each update that we're getting from Telegram.
	for update := range updates {

		// Check membership in native group
		status := membership[updateFrom(&update).ID]
		if statuses[status] == "" {

			if err := addNewUser(bot, updateFrom(&update)); err != nil {
				log.Panic(err)
			}

			if err := fillMembershipMap(); err != nil {
				log.Panic(err)
			}

			continue

		} else if statuses[status] != "valid" {

			if err := fillMembershipMap(); err != nil {
				log.Panic(err)
			}

			continue
		}

		if update.Message != nil {

			// Handle participant messages
			if update.Message.NewChatMembers != nil {
				addNewUsers(bot, update.Message.NewChatMembers)
			}

			if update.Message.LeftChatMember != nil {
				leftUser(bot, update.Message.LeftChatMember)
			}

			//Handle commands
			command := update.Message.Command()
			if command == "start" {

				// Fill and update libraryForReview
				var dictionary Dictionary

				if err := libraryCollection.FindOne(
					context.TODO(),
					bson.M{"ownerId": updateFrom(&update).ID}).Decode(&dictionary); err != nil {
					log.Panic(err)
				}

				libraryForReview[updateFrom(&update).ID] = dictionary.FactSet

				showHelp(bot, update)

			} else if command == "help" {
				showHelp(bot, update)

			} else if command == "settings" {
				showSettings(bot, update)

				// Add command hear

			} else if update.Message.IsCommand() {
				showMessage(bot, update, "Unrecognized command. Use /help.")
			}

			// Handle file
			if waitingForPushVocab {
				if update.Message.Document != nil {
					if update.Message.Document.MimeType == "text/csv" || update.Message.Document.MimeType == "text/comma-separated-values" {
						// Pushed .csv file

						// Get file direct url
						fileDirectUrl, err := bot.GetFileDirectURL(update.Message.Document.FileID)
						if err != nil {
							log.Panic(err)
						}

						// Make dir and download file
						os.Mkdir(string(rune(update.Message.From.ID)), os.ModePerm)
						csvDictionaryPath := "./" + string(rune(updateFrom(&update).ID)) + "/" + update.Message.Document.FileName
						downloadFile(fileDirectUrl, csvDictionaryPath)

						// Push dict to postgress
						err = addDictionary(csvDictionaryPath, update.Message.From)
						if err != nil {
							log.Panic(err)
						}
						// Reset waiting bool
						waitingForPushVocab = false

						showMessage(bot, update, "Vocabulary pushed.")
						showHelp(bot, update)

					} else {
						// Pushed not .csv file
						showMessage(bot, update, "Your file is not .csv. Sent please .csv file.")
					}
				} else {
					// Pushed something but not file
					showMessage(bot, update, "Still waiting for your own dictionary .csv file.")
				}
			} else if !waitingForPushVocab && update.Message.Document != nil {
				// Pushed file but not in time
				showMessage(bot, update, "For pushing your dictionary use /pushVocab")
			} // handle commands

		} else if update.CallbackQuery != nil {

			// Handle key pressing
			callback := update.CallbackQuery.Data

			// Pressed key Quiz
			if callback == "quiz" {
				log.Printf("In quiz case")
				nextQuestion(bot, update)
			}

			// Pressed key with correct answer
			if callback == "correctAnswer" {
				log.Printf("correctAnswer")
				nextQuestion(bot, update)
			}

			// Pressed key with incorrect answer
			if callback == "incorrectAnswer" {
				log.Printf("incorrectAnswer")
				nextQuestion(bot, update)
			}
			// Newbie check
			// Pressed key Settings
			if callback == "settings" {
				msg := tgbotapi.NewEditMessageText(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, "What do you want to set?")
				kbrd := tgbotapi.NewEditMessageReplyMarkup(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, settingsKeyboard)
				bot.Send(msg)
				bot.Send(kbrd)

			}
			// Pressed key Puss
			if callback == "pushVocab" {
				showMessage(bot, update, "Waiting for your own dictionary .csv file.")
				waitingForPushVocab = true
			}
			// Pressed key << Back
			if callback == "back" {
				msg := tgbotapi.NewEditMessageText(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, help)
				kbrd := tgbotapi.NewEditMessageReplyMarkup(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, mainMenuKeyboard)
				bot.Send(msg)
				bot.Send(kbrd)

			}
		}
	}
}

func showHelp(bot *tgbotapi.BotAPI, update tgbotapi.Update) error {

	var msg tgbotapi.MessageConfig

	if update.Message != nil {
		msg = tgbotapi.NewMessage(update.Message.Chat.ID, help)
	}

	if update.CallbackQuery != nil {
		msg = tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, help)
	}

	msg.ReplyMarkup = mainMenuKeyboard
	if _, err := bot.Send(msg); err != nil {
		log.Panic(err)
		return err
	}

	return nil
}

func showMessage(bot *tgbotapi.BotAPI, update tgbotapi.Update, message string) error {

	var msg tgbotapi.MessageConfig

	if update.Message != nil {
		msg = tgbotapi.NewMessage(update.Message.Chat.ID, message)
	}

	if update.CallbackQuery != nil {
		msg = tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, message)
	}

	if _, err := bot.Send(msg); err != nil {
		log.Panic(err)
		return err
	}

	return nil
}

func showSettings(bot *tgbotapi.BotAPI, update tgbotapi.Update) error {

	var msg tgbotapi.MessageConfig
	var message string = "What do you want to set?"

	if update.Message != nil {
		msg = tgbotapi.NewMessage(update.Message.Chat.ID, message)
	}

	if update.CallbackQuery != nil {
		msg = tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, message)
	}

	msg.ReplyMarkup = settingsKeyboard
	if _, err := bot.Send(msg); err != nil {
		log.Panic(err)
		return err
	}

	return nil
}

func nextQuestion(bot *tgbotapi.BotAPI, update tgbotapi.Update) {

	forReview := libraryForReview[updateFrom(&update).ID]
	count := countForReview[updateFrom(&update).ID]

	if count < len(forReview) {

		// Prepare randomized answer keybord
		randomOfFour := rand.Intn(3)
		randomAnswerArray := make([][]string, 4)
		for i := range randomAnswerArray {

			limiter := 0

			if count < 3 {
				limiter = 3
			}

			if count >= len(forReview)-3 {
				limiter = -3
			}

			framer := count - randomOfFour + i + limiter
			randomAnswerArray[i] = []string{forReview[framer].Answer, "incorrectAnswer"}
		}

		randomAnswerArray[randomOfFour] = []string{forReview[count].Answer, "correctAnswer"}

		var quizKeyboard = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(randomAnswerArray[0][0], randomAnswerArray[0][1]),
				tgbotapi.NewInlineKeyboardButtonData(randomAnswerArray[1][0], randomAnswerArray[1][1]),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(randomAnswerArray[2][0], randomAnswerArray[2][1]),
				tgbotapi.NewInlineKeyboardButtonData(randomAnswerArray[3][0], randomAnswerArray[3][1]),
			),
		) // prepare randomized answer keyboard

		msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, forReview[count].Question)
		msg.ReplyMarkup = quizKeyboard
		bot.Send(msg)
		countForReview[updateFrom(&update).ID]++

		//start := time.Now()

		log.Printf("readQuality(&update): %v\n", readQuality(&update))

	} else {

		countForReview[updateFrom(&update).ID] = 0
		showMessage(bot, update, "Finished!")
		showHelp(bot, update)
	}

}

func readQuality(update *tgbotapi.Update) int {

	sw := stopwatch[updateFrom(update).ID]

	if countForReview[updateFrom(update).ID] != 0 {
		sw.mark = time.Since(sw.start)
		sw.start = time.Now()

	} else {
		sw.mark = 0
		sw.start = time.Now()
	}

	log.Printf("sw.mark: %v\n", sw.mark)

	stopwatch[updateFrom(update).ID] = sw

	if update.CallbackQuery.Data == "correctAnswer" {

		if sw.mark.Seconds() < 5 {
			quality[updateFrom(update).ID] = 5

		} else if sw.mark.Seconds() > 5 && sw.mark.Seconds() < 10 {
			quality[updateFrom(update).ID] = 4

		} else if sw.mark.Seconds() > 10 {
			quality[updateFrom(update).ID] = 3
		}

	} else if update.CallbackQuery.Data != "correctAnswer" {

		if sw.mark.Seconds() < 5 {
			quality[updateFrom(update).ID] = 2

		} else if sw.mark.Seconds() > 5 {
			quality[updateFrom(update).ID] = 1
		}
	}

	return quality[updateFrom(update).ID]
}

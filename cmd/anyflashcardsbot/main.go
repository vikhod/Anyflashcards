package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/burke/nanomemo/supermemo"
	"github.com/go-co-op/gocron"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

var help = `
I can help you to remember a lot of the new words.
Lern words with your own dictionary.
	
You can control me by sending commands:
/start - Start bot
/help - Show this help
/quiz - Start learning
/hot20 - Repeat 20 random words from your dictionary
/settings - Configure bot parameters
/pushdict - Push your own dictionary
/pulldict - Pull your own dictionary
/settime - Set reminder time
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
		tgbotapi.NewInlineKeyboardButtonData("Set reminder time", "setReminder"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("<< Back", "back"),
	),
)

var (
	libraryForReview = map[int]supermemo.FactSet{}
	indexForReview   = map[int]int{}
	membership       = map[int]string{}
	reminderChart    = map[int]string{}
)

type Stopwatch struct {
	start time.Time
	mark  time.Duration
}

var (
	stopwatch = map[int]Stopwatch{}
	quality   = map[int]int{}
)

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
	waitingForSetReminder := false

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

	// Fill reminderChart map for remingding
	setAllReminds(*bot)

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

			// Handle commands
			command := update.Message.Command()
			if command == "start" {

				showHelp(bot, update)
			} else if command == "help" {

				showHelp(bot, update)
			} else if command == "settings" {

				showSettings(bot, update)

				// Add commands hear

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
						os.Mkdir(strconv.Itoa(updateFrom(&update).ID), os.ModePerm)
						csvDictionaryPath := "./" + strconv.Itoa(updateFrom(&update).ID) + "/" + update.Message.Document.FileName
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

				showMessage(bot, update, "For pushing your dictionary use /pushVocab")
			}

			// Handle time for seting reminder
			if waitingForSetReminder {
				setReminder(updateFrom(&update), update.Message.Text)
				setAllReminds(*bot)
				waitingForSetReminder = false
			}

		} else if update.CallbackQuery != nil {

			// Handle key pressing
			callback := update.CallbackQuery.Data

			if callback == "quiz" {

				indexForReview[updateFrom(&update).ID] = 0
				stopwatch[updateFrom(&update).ID] = Stopwatch{}

				// Update libraryForReview
				factSet, err := loadFactsFromBase(updateFrom(&update))
				if err != nil {
					log.Panic(err)
				}
				smFactSet := toSupermemoFactSet(&factSet)
				libraryForReview[updateFrom(&update).ID] = smFactSet.ForReview()

				nextQuestion(bot, update)
			}

			if callback == "correctAnswer" {
				nextQuestion(bot, update)
			}

			if callback == "incorrectAnswer" {
				nextQuestion(bot, update)
			}

			// Newbie check (maybe add later)

			if callback == "settings" {
				msg := tgbotapi.NewEditMessageText(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, "What do you want to set?")
				kbrd := tgbotapi.NewEditMessageReplyMarkup(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, settingsKeyboard)
				bot.Send(msg)
				bot.Send(kbrd)

			}

			if callback == "pushVocab" {
				showMessage(bot, update, "Waiting for your own dictionary .csv file.")
				waitingForPushVocab = true
			}

			if callback == "setReminder" {
				showMessage(bot, update, "Waiting for your time string. Send string like 20:00.")
				waitingForSetReminder = true
			}

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
	index := indexForReview[updateFrom(&update).ID]

	// Make slice for randomization
	forRandomization := make(supermemo.FactSet, len(forReview))
	copy(forRandomization, forReview)

	if len(forReview) > 0 {

		if index < len(forReview) {

			forRandomization = append(forRandomization[:index], forRandomization[index+1:]...)
			arrayOfFourPosibleAnswer := make([][]string, 4)

			// Fill slice for randomization
			for i := 0; i < 4; i++ {

				arrayOfFourPosibleAnswer[i] = []string{"   ...   ", "incorrectAnswer"}

				if len(forRandomization) > 0 {
					randomPosition := rand.Intn(len(forRandomization))
					arrayOfFourPosibleAnswer[i] = []string{forRandomization[randomPosition].Answer, "incorrectAnswer"}
					forRandomization = append(forRandomization[:randomPosition], forRandomization[randomPosition+1:]...)
				}
			}

			// Prepare randomized answer keybord
			randomOfFour := rand.Intn(3)
			arrayOfFourPosibleAnswer[randomOfFour] = []string{forReview[index].Answer, "correctAnswer"}

			quizKeyboard := tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(arrayOfFourPosibleAnswer[0][0], arrayOfFourPosibleAnswer[0][1]),
					tgbotapi.NewInlineKeyboardButtonData(arrayOfFourPosibleAnswer[1][0], arrayOfFourPosibleAnswer[1][1]),
				),
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(arrayOfFourPosibleAnswer[2][0], arrayOfFourPosibleAnswer[2][1]),
					tgbotapi.NewInlineKeyboardButtonData(arrayOfFourPosibleAnswer[3][0], arrayOfFourPosibleAnswer[3][1]),
				),
			)

			// Show question with randomized keyboard
			msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, forReview[index].Question)
			msg.ReplyMarkup = quizKeyboard
			bot.Send(msg)

			// Read quality of answer with usin stopwatch
			quality := readQuality(&update)

			if index > 0 {
				forReview[index-1].Assess(quality)
			}

			indexForReview[updateFrom(&update).ID]++

		} else if index == len(forReview) {

			// Read last update
			quality := readQuality(&update)
			forReview[index-1].Assess(quality)

			// Nullify variables
			indexForReview[updateFrom(&update).ID] = 0
			stopwatch[updateFrom(&update).ID] = Stopwatch{}

			// Dump facts into base
			factSet := toFactSet(&forReview)
			if err := updateFactsInBase(updateFrom(&update), &factSet); err != nil {
				log.Printf("err: %v\n", err.Error())
			}

			// Update library for review
			libraryForReview[updateFrom(&update).ID] = forReview.ForReview()

			// Run nextQustion
			if len(libraryForReview[updateFrom(&update).ID]) > 1 {
				nextQuestion(bot, update)
			} else {
				showMessage(bot, update, "Finished!")
				showHelp(bot, update)
			}

		}

	} else {
		showMessage(bot, update, "Nothing for repetition today! Try Hot20.")
	}
}

func readQuality(update *tgbotapi.Update) int {
	sw := stopwatch[updateFrom(update).ID]

	if indexForReview[updateFrom(update).ID] != 0 {
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

	} else if update.CallbackQuery.Data == "incorrectAnswer" {
		if sw.mark.Seconds() < 5 {
			quality[updateFrom(update).ID] = 2
		} else if sw.mark.Seconds() > 5 {
			quality[updateFrom(update).ID] = 1
		}

	} else if update.CallbackQuery.Data == "blackout" {
		quality[updateFrom(update).ID] = 0
	}

	return quality[updateFrom(update).ID]
}

func remind(bot tgbotapi.BotAPI, userId int64) {
	log.Printf("\"inRemind\": %v\n", "inRemind")
	msg := tgbotapi.NewMessage(userId, "Time to go. Press Quiz!")

	if _, err := bot.Send(msg); err != nil {
		log.Panic(err)
	}
}

var scheduler gocron.Scheduler

//var task = remind()

func setAllReminds(bot tgbotapi.BotAPI) {

	reminderChart, err := getAllReminds()
	if err != nil {
		log.Panic(err)
	}

	for remindId, remindString := range reminderChart {

		location, _ := time.LoadLocation("Europe/Kiev")
		scheduler = *gocron.NewScheduler(location)

		var task = func() { remind(bot, int64(remindId)) }

		scheduler.Every(1).Day().At(remindString).Do(task)
		fmt.Printf("remindId: %v\n", remindId)
		fmt.Printf("remindString: %v\n", remindString)
		fmt.Printf("location: %v\n", location.String())
		scheduler.StartAsync()

	}
}

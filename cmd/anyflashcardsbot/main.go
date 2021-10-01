package main

import (
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/burke/nanomemo/supermemo"
	"github.com/go-co-op/gocron"
	"go.mongodb.org/mongo-driver/bson/primitive"

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
		tgbotapi.NewInlineKeyboardButtonData("Hot20", "hot20"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Settings", "settings"),
	),
)

var settingsKeyboard = tgbotapi.NewInlineKeyboardMarkup(
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Pick dictionary", "pickDict"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Push dictionary", "pushDict"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Pull dictionary", "pullDict"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Set reminder time", "setRemTime"),
	),
	tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("<< Back", "backToMain"),
	),
)

var (
	libraryForReview = map[int]supermemo.FactSet{}
	indexForReview   = map[int]int{}
	membership       = map[int]string{}
	remindsChart     = map[int]string{}
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
var defaultDictionaryName = "owsi.csv"
var defaultDictionaryId primitive.ObjectID

// Set waiting bool
//var waitingForDictionaryID = false
var waitingForDictionaryFile = false
var waitingForTime = false

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

	// Connect to database
	if err := connectMongoDb(); err != nil {
		log.Panic(err)
	}

	// Create and fill default library in database
	if err := updateDefaultLibrary(defaultLibraryDirPath); err != nil {
		log.Panic(err)
	}

	// Fill users map for security checking
	if membership, err = loadAllUsersStatusFromBase(); err != nil {
		log.Panic(err)
	}

	// Add anyflashcardsbot user to database
	if err = addNewUser(bot, &bot.Self); err != nil {
		log.Panic(err)
	}

	// Fill reminderChart map for remingding
	setAllReminds(bot)

	// Go through each update that we're getting from Telegram.
	for update := range updates {

		if !checkMembership(bot, &update) {
			continue
		}

		if update.Message != nil {

			// Handle membership messages
			if update.Message.NewChatMembers != nil {
				log.Printf("\"update.Message.NewChatMembers != nil\": %v\n", "update.Message.NewChatMembers != nil")
				addNewUsers(bot, update.Message.NewChatMembers)
				destinationId, err := copyDictionaryInBase(&defaultDictionaryId)
				var meta = DictionaryMetadata{
					Date:    time.Now(),
					OwnerID: update.Message.From.ID,
					Status:  "current"}

				if err != nil {
					log.Printf("err: %v\n", err)
				}
				if err := setDictionaryMetaInBase(destinationId, meta); err != nil {
					log.Printf("err: %v\n", err)
				}
			}
			if update.Message.LeftChatMember != nil {
				blockUser(bot, update.Message.LeftChatMember.ID)
				if membership, err = loadAllUsersStatusFromBase(); err != nil {
					log.Printf("err: %v\n", err)
				}
			}

			// Handle commands
			command := update.Message.Command()
			if command == "start" {
				showMainMeny(bot, update.Message.From.ID)

			} else if command == "help" {
				showMainMeny(bot, update.Message.From.ID)

			} else if command == "settings" {
				showSettings(bot, update)

				// Add commands hear

			} else if update.Message.IsCommand() {
				showMessage(bot, update.Message.From.ID, "Unrecognized command. Use /help.")
			}

			// Handle file
			if waitingForDictionaryFile {
				if _id, err := pushDictionaryToBase(bot, &update); err != nil {
					log.Printf("err: %v\n", err)
				} else {
					if err = organizePrivateUserDictionariesInBase(update.Message.From.ID); err != nil {
						log.Printf("err: %v\n", err)
					}
					var meta = DictionaryMetadata{Date: time.Now(), OwnerID: update.Message.From.ID, Status: "current"}
					if err = setDictionaryMetaInBase(_id, meta); err != nil {
						log.Printf("err: %v\n", err)
					}
				}

			} else if !waitingForDictionaryFile && update.Message.Document != nil {

				showMessage(bot, update.Message.From.ID, "For pushing your dictionary use /pushDict")
			}

			// Handle time for seting reminder
			if waitingForTime {
				dumpReminderToBase(update.Message.From.ID, update.Message.Text)
				setAllReminds(bot)
				waitingForTime = false
			}

		} else if update.CallbackQuery != nil {

			// Handle key pressing
			callback := update.CallbackQuery.Data

			if callback == "quiz" {

				indexForReview[update.CallbackQuery.From.ID] = 0
				stopwatch[update.CallbackQuery.From.ID] = Stopwatch{}

				// Update libraryForReview
				factSet, err := loadFactsFromBase(update.CallbackQuery.From)
				if err != nil {
					log.Panic(err)
				}
				smFactSet := convertToSupermemoFactSet(&factSet)
				libraryForReview[update.CallbackQuery.From.ID] = smFactSet.ForReview()

				nextQuestion(bot, update.CallbackQuery.From.ID, update.CallbackQuery.Data)
			}

			if callback == "correctAnswer" {
				forReview := libraryForReview[update.CallbackQuery.From.ID]
				index := indexForReview[update.CallbackQuery.From.ID] - 1
				msg := "Right!: " + forReview[index].Question + " - " + forReview[index].Answer
				calbackAnswer := tgbotapi.NewCallbackWithAlert(update.CallbackQuery.ID, msg)
				bot.AnswerCallbackQuery(calbackAnswer)
				nextQuestion(bot, update.CallbackQuery.From.ID, update.CallbackQuery.Data)

			}

			if callback == "incorrectAnswer" {
				forReview := libraryForReview[update.CallbackQuery.From.ID]
				index := indexForReview[update.CallbackQuery.From.ID] - 1
				msg := "Wrong!: " + forReview[index].Question + " - " + forReview[index].Answer
				calbackAnswer := tgbotapi.NewCallbackWithAlert(update.CallbackQuery.ID, msg)
				bot.AnswerCallbackQuery(calbackAnswer)
				nextQuestion(bot, update.CallbackQuery.From.ID, update.CallbackQuery.Data)
			}

			// Newbie check (maybe add later)

			if callback == "settings" {
				showSettings(bot, update)
			}

			if callback == "pickDict" {
				showPickDictKeyboard(bot, update.CallbackQuery.From.ID)
				//waitingForDictionaryID = true
			}

			if primitive.IsValidObjectID(callback) {
				dictionaryId, _ := primitive.ObjectIDFromHex(callback)
				resultDictionaryId, _ := copyDictionaryInBase(&dictionaryId)
				organizePrivateUserDictionariesInBase(update.CallbackQuery.From.ID)

				var meta = DictionaryMetadata{
					Date:     time.Now(),
					FilePath: callback,
					OwnerID:  update.CallbackQuery.From.ID,
					Status:   "current"}

				setDictionaryMetaInBase(resultDictionaryId, meta)
				showPickDictKeyboard(bot, update.CallbackQuery.From.ID)

			}

			if callback == "pushDict" {
				showMessage(bot, update.CallbackQuery.From.ID, "Waiting for your own dictionary .csv file.")
				waitingForDictionaryFile = true
			}

			if callback == "setRemTime" {
				showMessage(bot, update.CallbackQuery.From.ID, "Waiting for your time string. Send string like 20:00.")
				waitingForTime = true
			}

			if callback == "backToMain" {
				msg := tgbotapi.NewEditMessageText(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, help)
				kbrd := tgbotapi.NewEditMessageReplyMarkup(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, mainMenuKeyboard)
				bot.Send(msg)
				bot.Send(kbrd)

			}
			if callback == "backToSettings" {
				msg := tgbotapi.NewEditMessageText(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, "What do you want to set?")
				kbrd := tgbotapi.NewEditMessageReplyMarkup(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, settingsKeyboard)
				bot.Send(msg)
				bot.Send(kbrd)

			}

		}
	}
}

func showMainMeny(bot *tgbotapi.BotAPI, userId int) error {

	msg := tgbotapi.NewMessage(int64(userId), help)

	msg.ReplyMarkup = mainMenuKeyboard
	if _, err := bot.Send(msg); err != nil {
		log.Panic(err)
		return err
	}

	return nil
}

func showMessage(bot *tgbotapi.BotAPI, userId int, message string) error {

	msg := tgbotapi.NewMessage(int64(userId), message)

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

func showPickDictKeyboard(bot *tgbotapi.BotAPI, userId int) error {

	publicDictionaries, err := loadAllPublicDictionaryFromBase()
	if err != nil {
		return err
	}
	privateUserDictionaries, err := loadAllUsersDictionariesFromBase(userId)
	if err != nil {
		return err
	}
	availableDictionaries := append(publicDictionaries, privateUserDictionaries...)

	msg := tgbotapi.NewMessage(int64(userId), "Pick your dictionary:")
	pickDictKeyboard := tgbotapi.NewInlineKeyboardMarkup()

	for _, dictionary := range availableDictionaries {
		var row []tgbotapi.InlineKeyboardButton
		btnText := "Name: " + dictionary.DictionaryMetadata.Name +
			"\nStatus:" + dictionary.DictionaryMetadata.Status +
			"\nDate:" + dictionary.DictionaryMetadata.Date.Local().Format("2006-January-02 15:04:05")
		btn := tgbotapi.NewInlineKeyboardButtonData(btnText, dictionary.ID.Hex())
		row = append(row, btn)
		pickDictKeyboard.InlineKeyboard = append(pickDictKeyboard.InlineKeyboard, row)
	}
	backBtn := tgbotapi.NewInlineKeyboardButtonData("<< Back", "backToSettings")
	var row []tgbotapi.InlineKeyboardButton
	row = append(row, backBtn)
	pickDictKeyboard.InlineKeyboard = append(pickDictKeyboard.InlineKeyboard, row)
	msg.ReplyMarkup = pickDictKeyboard

	if _, err = bot.Send(msg); err != nil {
		return err
	}

	return nil
}

func showAnswerKeybord(bot *tgbotapi.BotAPI, userId int) error {

	forReview := libraryForReview[userId]
	index := indexForReview[userId]

	log.Printf("len(forReview): %v\n", len(forReview))
	log.Printf("index: %v\n", index)

	// Make slice for randomization
	forRandomization := make(supermemo.FactSet, len(forReview))
	copy(forRandomization, forReview)

	forRandomization = append(forRandomization[:index], forRandomization[index+1:]...)
	arrayOfFourPosibleAnswer := make([][]string, 4)

	// Fill slice for randomization
	for i := 0; i < 4; i++ {

		log.Printf("i: %v\n", i)
		arrayOfFourPosibleAnswer[i] = []string{"   ...   ", "incorrectAnswer"}

		if len(forRandomization) > 0 {
			randomPosition := rand.Intn(len(forRandomization))
			arrayOfFourPosibleAnswer[i] = []string{forRandomization[randomPosition].Answer, "incorrectAnswer"}
			forRandomization = append(forRandomization[:randomPosition], forRandomization[randomPosition+1:]...)
		}
	}

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
	msg := tgbotapi.NewMessage(int64(userId), forReview[index].Question)
	msg.ReplyMarkup = quizKeyboard
	if _, err := bot.Send(msg); err != nil {
		return err
	}

	return nil
}

func nextQuestion(bot *tgbotapi.BotAPI, userId int, callbackQueryData string) {

	forReview := libraryForReview[userId]
	index := indexForReview[userId]

	// Make slice for randomization
	forRandomization := make(supermemo.FactSet, len(forReview))
	copy(forRandomization, forReview)

	if len(forReview) > 0 {

		if index < len(forReview) {

			if err := showAnswerKeybord(bot, userId); err != nil {
				log.Panic(err)
			}

			// Read quality of answer with usin stopwatch
			quality := readQuality(userId, callbackQueryData)

			if index > 0 {
				forReview[index-1].Assess(quality)
			}

			indexForReview[userId]++

		} else if index == len(forReview) {

			// Read last update
			quality := readQuality(userId, callbackQueryData)
			forReview[index-1].Assess(quality)

			// Nullify variables
			indexForReview[userId] = 0
			stopwatch[userId] = Stopwatch{}

			// Dump facts into base
			factSet := convertToFactSet(&forReview)
			if err := updateFactsInBase(userId, &factSet); err != nil {
				log.Printf("err: %v\n", err.Error())
			}

			// Update library for review
			libraryForReview[userId] = forReview.ForReview()

			// Run nextQustion
			if len(libraryForReview[userId]) > 1 {
				nextQuestion(bot, userId, callbackQueryData)
			} else {
				showMessage(bot, userId, "Finished!")
				showMainMeny(bot, userId)
			}
		}

	} else {

		showMessage(bot, userId, "Nothing for repetition today! Try Hot20.")
	}
}

func readQuality(userId int, calbackQueryData string) int {
	sw := stopwatch[userId]

	if indexForReview[userId] != 0 {
		sw.mark = time.Since(sw.start)
		sw.start = time.Now()

	} else {
		sw.mark = 0
		sw.start = time.Now()
	}

	log.Printf("sw.mark: %v\n", sw.mark)

	stopwatch[userId] = sw

	if calbackQueryData == "correctAnswer" {
		if sw.mark.Seconds() < 5 {
			quality[userId] = 5
		} else if sw.mark.Seconds() > 5 && sw.mark.Seconds() < 10 {
			quality[userId] = 4
		} else if sw.mark.Seconds() > 10 {
			quality[userId] = 3
		}

	} else if calbackQueryData == "incorrectAnswer" {
		if sw.mark.Seconds() < 5 {
			quality[userId] = 2
		} else if sw.mark.Seconds() > 5 {
			quality[userId] = 1
		}

	} else if calbackQueryData == "blackout" {
		quality[userId] = 0
	}

	return quality[userId]
}

func showRemind(bot tgbotapi.BotAPI, userId int64) {

	msg := tgbotapi.NewMessage(userId, "Time to go! Press `Quiz.`")

	if _, err := bot.Send(msg); err != nil {
		log.Panic(err)
	}
}

var scheduler gocron.Scheduler // Check nesesarity
func setAllReminds(bot *tgbotapi.BotAPI) {

	var err error
	remindsChart, err = loadAllRemindsFromBase()
	if err != nil {
		log.Panic(err)
	}

	for userId, remindTime := range remindsChart {

		location, _ := time.LoadLocation("Europe/Kiev")
		scheduler = *gocron.NewScheduler(location)

		var task = func() { showRemind(*bot, int64(userId)) }

		scheduler.Every(1).Day().At(remindTime).Do(task)
		scheduler.StartAsync()

	}
}

/*
Done:
* TODO Div function showHelp and function showMainKeyboard
* TODO Rewrite file handler
* TODO Clean db functions, dell addDictionary
* TODO Cut out function getUpdateInitiator, maybe mix functionality with checkMembership - Done. Check with other users.
* TODO Rewrite security function
* TODO Add taking id for each dump function
* TODO Add returning id into setDefDictInBase and dell getDefDict
* TODO Add to organize function deleting user dicts if more than three
* TODO Add function chouse dictionary - need to be repaired
* TODO Add showing available dictionaries for user (and public and his private)
* TODO Add correct answer into each callback message

In work:
* TODO Create helm chart and run it somewere

In plan:
* TODO Add function for download dictionary
* TODO Add posibiliti for pickDictionary pick private dict without copying
* TODO Add exeptions into time handler
* TODO Unite all map in one map or struct
* TODO Add stop key into Quiz
* TODO Add blackout key into Quiz
* TODO Add function for chouse location
* TODO Rewrite all messages wih html
* TODO Add functionalyty for send message to creator in error situation
* TODO Move it up, all hardcoded strings
*/

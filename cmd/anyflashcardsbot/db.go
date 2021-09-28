package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/burke/nanomemo/supermemo"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	mongo_server                = os.Getenv("MONGO_SERVER")
	mongo_port                  = os.Getenv("MONGO_PORT")
	mongo_inintdb_root_username = os.Getenv("MONGO_INITDB_ROOT_USERNAME")
	mongo_inintdb_root_password = os.Getenv("MONGO_INITDB_ROOT_PASSWORD")
	mongo_url                   = fmt.Sprintf("mongodb://%s:%s@%s:%s", mongo_inintdb_root_username, mongo_inintdb_root_password, mongo_server, mongo_port)
)

var (
	Client            *mongo.Client
	database          *mongo.Database
	libraryCollection *mongo.Collection
	usersCollection   *mongo.Collection
)

func connectMongoDb() error {
	// Connect to MongoDB Default '20:00'
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(mongo_url))
	if err != nil {
		return err
	}
	Client = client

	database = Client.Database("anyflashcardsbot")
	libraryCollection = database.Collection("library")
	usersCollection = database.Collection("users")

	// Check the connection
	err = Client.Ping(context.TODO(), nil)
	if err != nil {
		return err
	}
	log.Printf("MongoDB server connected!")
	return nil
}

type User struct {
	ID               primitive.ObjectID  `bson:"_id"`
	User             tgbotapi.User       `bson:"user"`
	NativeChatMember tgbotapi.ChatMember `bson:"native_chat_member"`
	Dictionary       string              `bson:"dictionary"`
	ReminderTime     string              `bson:"reminder_time"`
}

/*
func loadAllUsersFromBase() (users []User, err error) {

	//statuses := map[int]string{}
	cursor, err := usersCollection.Find(context.TODO(), bson.M{})
	if err != nil {
		return nil, err
	}

	if err = cursor.All(context.TODO(), &users); err != nil {
		return nil, err
	}

	return users, nil
}
*/

func loadAllUsersStatusFromBase() (map[int]string, error) {

	statuses := map[int]string{}
	users, err := usersCollection.Find(context.TODO(), bson.M{})
	if err != nil {
		return nil, err
	}

	for users.Next(context.TODO()) {
		var user User
		if err = users.Decode(&user); err != nil {
			return nil, err
		}
		statuses[user.User.ID] = user.NativeChatMember.Status
	}

	return statuses, nil
}

func loadAllRemindsFromBase() (map[int]string, error) {
	reminds := map[int]string{}
	users, err := usersCollection.Find(context.TODO(), bson.M{})
	if err != nil {
		return nil, err
	}
	for users.Next(context.TODO()) {
		var user User
		if err = users.Decode(&user); err != nil {
			return nil, err
		}

		if user.ReminderTime != "" {
			reminds[user.User.ID] = user.ReminderTime
		}
	}
	return reminds, err
}

func dumpReminderToBase(userId int, time string) error {

	_, err := usersCollection.UpdateOne(
		context.TODO(),
		bson.M{"user.id": userId},
		bson.M{"$set": bson.M{"reminder_time": time}},
	)
	if err != nil {
		return err
	}

	return nil
}

var defaultDictionaryPath = "./configs/dictionaries/owsi.csv"

func addNewUsers(bot *tgbotapi.BotAPI, newUsers *[]tgbotapi.User) error {
	for _, newUser := range *newUsers {
		addNewUser(bot, &newUser)
	}

	return nil
}

var nativeGroupChatID, _ = strconv.ParseInt(os.Getenv("NATIVE_GROUP_CHAT_ID"), 10, 64)

func addNewUser(bot *tgbotapi.BotAPI, newUser *tgbotapi.User) error {
	// Create struct for new user
	var user User
	user.ID = primitive.NewObjectID()
	user.User = *newUser
	user.NativeChatMember.Status = "member"
	user.Dictionary = defaultDictionaryPath
	user.ReminderTime = ""

	// Create chat config for tgbot
	var chatConfigWithUser tgbotapi.ChatConfigWithUser
	chatConfigWithUser.ChatID = nativeGroupChatID
	chatConfigWithUser.UserID = newUser.ID

	// Get chat member info from native group
	chatMember, err := bot.GetChatMember(chatConfigWithUser)
	if err != nil {
		log.Panic(err)
	}

	// Add user into database if user isn't existent
	if err := usersCollection.FindOne(
		context.TODO(),
		bson.M{"user.id": &newUser.ID}); err.Err() == mongo.ErrNoDocuments {
		_, err := usersCollection.InsertOne(
			context.TODO(),
			user,
		)
		if err != nil {
			return err
		}

		_, err = usersCollection.UpdateOne(
			context.TODO(),
			bson.M{"user.id": &newUser.ID},
			bson.M{"$set": bson.M{"native_chat_member": chatMember}},
		)
		if err != nil {
			return err
		}

	} else if err.Err() != mongo.ErrNoDocuments {
		_, err := usersCollection.UpdateOne(
			context.TODO(),
			bson.M{"user.id": &newUser.ID},
			bson.M{"$set": bson.M{"native_chat_member": chatMember}},
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func blockUser(bot *tgbotapi.BotAPI, oustUserID int) error {
	var chatConfigWithUser tgbotapi.ChatConfigWithUser
	chatConfigWithUser.ChatID = nativeGroupChatID
	chatConfigWithUser.UserID = oustUserID

	chatMember, err := bot.GetChatMember(chatConfigWithUser)
	if err != nil {
		log.Panic(err)
	}

	_, err = usersCollection.UpdateOne(
		context.TODO(),
		bson.M{"user.id": oustUserID},
		bson.M{"$set": bson.M{"native_chat_member": chatMember}},
	)

	if err != nil {
		return err
	}

	return nil
}

type Dictionary struct {
	ID                 primitive.ObjectID `bson:"_id"`
	FactSet            FactSet            `bson:"factSet"`
	DictionaryMetadata DictionaryMetadata `bson:"dictionaryMetadata"`
}

type DictionaryMetadata struct {
	Name string `bson:"name"`
	Date string `bson:"date"`
	// Where dictionary was loaded from
	FilePath string `bson:"filePath"`
	OwnerID  int    `bson:"ownerId"`
	// Public or private, private is available only for owner
	Status string `bson:"status"`
	// Default dictionary for all users
}

type FactSet []Fact

type Fact struct {
	Question string
	Answer   string
	FactMetadata
}

type FactMetadata struct {
	// Easiness FactMetadataor of the fact. Higher means the item is easier for the user
	// to remember.
	Ef float64 `bson:"ef"`
	// Interval number of days to wait before presenting this item again after the
	// end of this session.
	Interval int `bson:"interval"`
	// last time the fact was reviewed. Interval counts days from here.
	IntervalFrom string `bson:"intervalFrom"`
	// number of times this fact has been presented; reset to 0 on failed answer.
	N int `bson:"n"`
}

func convertToSupermemoFactSet(factSet *FactSet) *supermemo.FactSet {

	var smFactSet supermemo.FactSet

	for _, fact := range *factSet {

		q := fact.Question
		a := fact.Answer
		ef := fact.Ef
		n := fact.N
		interval := fact.Interval
		intervalFrom := fact.IntervalFrom

		smFact, _ := supermemo.LoadFact(q, a, ef, n, interval, intervalFrom)

		smFactSet = append(smFactSet, smFact)

	}
	return &smFactSet
}

func convertToFactSet(smFactSet *supermemo.FactSet) FactSet {

	var factSet FactSet

	for _, smFact := range *smFactSet {

		q, a, ef, n, interval, intervalFrom := smFact.Dump()

		var fact Fact
		fact.Question = q
		fact.Answer = a
		fact.FactMetadata.Ef = ef
		fact.FactMetadata.N = n
		fact.FactMetadata.Interval = interval
		fact.FactMetadata.IntervalFrom = intervalFrom

		factSet = append(factSet, fact)
	}

	return factSet
}

func updateFactsInBase(userId int, factSet *FactSet) error {
	for _, fact := range *factSet {

		_, err := libraryCollection.UpdateOne(
			context.TODO(),
			bson.M{"dictionaryMetadata.ownerId": userId, "factSet.question": fact.Question},
			bson.M{"$set": bson.M{"factSet.$.factmetadata": fact.FactMetadata}},
		)
		if err != nil {

			return err
		}
	}

	return nil
}

func loadFactsFromBase(user *tgbotapi.User) (FactSet, error) {

	var dictionary Dictionary
	var err error
	if err = libraryCollection.FindOne(
		context.TODO(),
		bson.M{"dictionaryMetadata.ownerId": user.ID}).Decode(&dictionary); err != nil {

		log.Panic(err)
		return nil, err
	}

	return dictionary.FactSet, err
}

// Functions for Dictionary
func updateDefaultLibrary(defaultLibraryDirPath string) error {
	_, err := libraryCollection.DeleteMany(
		context.TODO(),
		bson.M{"$or": []interface{}{bson.M{"dictionaryMetadata.status": "library"}, bson.M{"dictionaryMetadata.status": "default"}}},
	)

	if err != nil {
		return err
	}

	csvDictionariesPathes, err := os.ReadDir(defaultLibraryDirPath)
	if err != nil {
		return err
	}

	for _, csvDictionaryPath := range csvDictionariesPathes {

		csvPath := defaultLibraryDirPath + "/" + csvDictionaryPath.Name()
		dictionary, err := readDictionaryFromDisc(csvPath)
		if err != nil {
			return err
		}

		dictionary.DictionaryMetadata.Status = "library"
		dumpDictionaryToBase(&dictionary)
	}

	return nil
}

var dictStatuses = map[string]string{
	"library": "public",  //Loaded from disc, from default library
	"public":  "public",  //Pushed by administrator or copied by adminstrator from another one
	"default": "public",  //Default dict for new users, from default library
	"private": "private", //Pused by user, but isn't used now
	"current": "private", //Picked or pushed by user and is used now
}

func loadAllPublicDictionaryFromBase() (dictionaries []Dictionary, err error) {

	dictionaryCursor, err := libraryCollection.Find(context.TODO(), bson.M{})
	if err != nil {
		return nil, err
	}
	for dictionaryCursor.Next(context.TODO()) {
		var dictionary Dictionary
		if err = dictionaryCursor.Decode(&dictionary); err != nil {
			return nil, err
		}

		if dictStatuses[dictionary.DictionaryMetadata.Status] == "public" {
			dictionaries = append(dictionaries, dictionary)
		}
	}
	return dictionaries, nil
}

func loadDictionaryFromBase(dictionaryId *primitive.ObjectID) (dictionary Dictionary, err error) {

	dictionaryCursor, err := libraryCollection.Find(context.TODO(), bson.M{"_id": *dictionaryId})
	if err != nil {
		return dictionary, err
	}

	for dictionaryCursor.Next(context.TODO()) {
		if err = dictionaryCursor.Decode(&dictionary); err != nil {
			return dictionary, err
		}
	}

	return dictionary, nil
}

func dumpDictionaryToBase(dictionary *Dictionary) (*primitive.ObjectID, error) {

	dictionary.ID = primitive.NewObjectID()

	_, err := libraryCollection.InsertOne(
		context.TODO(),
		dictionary,
	)
	if err != nil {
		return nil, err
	}

	return &dictionary.ID, nil
}

func copyDictionaryInBase(sourceId *primitive.ObjectID) (*primitive.ObjectID, error) {

	dictionary, err := loadDictionaryFromBase(sourceId)
	if err != nil {
		return nil, err
	}

	//dictionary.DictionaryMetadata.Status = "current"
	dictionary.DictionaryMetadata.FilePath = dictionary.ID.Hex()

	resultId, err := dumpDictionaryToBase(&dictionary)
	if err != nil {
		return nil, err
	}

	return resultId, nil
}

func setDictionaryStatusInBase(dictionaryId *primitive.ObjectID, status string) error {

	result, err := libraryCollection.UpdateOne(
		context.TODO(),
		bson.M{"_id": dictionaryId},
		bson.M{"$set": bson.M{"dictionaryMetadata.status": status}},
	)
	log.Printf("dictionaryId: %v\n", dictionaryId)
	log.Printf("result: %v\n", result)
	if err != nil {
		return err
	}
	return nil
}

func setDefaultDictionaryInBase(name string) (_id primitive.ObjectID, err error) {

	var dictionary Dictionary
	err = libraryCollection.FindOneAndUpdate(
		context.TODO(),
		bson.M{"dictionaryMetadata.name": name, "dictionaryMetadata.status": "library"},
		bson.M{"$set": bson.M{"dictionaryMetadata.status": "default"}},
	).Decode(&dictionary)
	if err != nil {
		return _id, err
	}
	return dictionary.ID, nil
}

func organizePrivateUserDictionariesInBase(userId int) (err error) {

	if _, err = libraryCollection.UpdateMany(
		context.TODO(),
		bson.M{"dictionaryMetadata.ownerId": userId},
		bson.M{"$set": bson.M{"dictionaryMetadata.status": "private"}},
	); err != nil {
		return err
	}

	return nil
}

/*
Functions list:

connectMongoDb - OK

loadAllUsersStatusFromBase - OK
loadAllRemindsFromBase     - OK
dumpReminderToBase - OK

addNewUsers
addNewUser
deleteUser

updateDefaultLibrary
updateFactsInBase
loadFactsFromBase
dumpDictionaryToBase

convertToSupermemoFactSet
convertToFactSet
*/

/*
Done:
* TODO Add returning id into setDefDictInBase and dell getDefDict

In plan:
*
*
*/

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
	// Connect to MongoDB
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

var nativeGroupChatID, _ = strconv.ParseInt(os.Getenv("NATIVE_GROUP_CHAT_ID"), 10, 64)

func fillMembershipMap() error {
	users, err := usersCollection.Find(context.TODO(), bson.M{})
	if err != nil {
		return err
	}

	for users.Next(context.TODO()) {
		var user User
		if err = users.Decode(&user); err != nil {
			return err
		}
		membership[user.User.ID] = user.NativeChatMember.Status
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

func addNewUser(bot *tgbotapi.BotAPI, newUser *tgbotapi.User) error {
	// Create struct for new user
	var user User
	user.ID = primitive.NewObjectID()
	user.User = *newUser
	user.NativeChatMember.Status = "member"
	user.Dictionary = defaultDictionaryPath
	user.ReminderTime = "9:10"

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

		addDictionary(defaultDictionaryPath, newUser)

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

func setReminder(user *tgbotapi.User, time string) error {

	_, err := usersCollection.UpdateOne(
		context.TODO(),
		bson.M{"user.id": &user.ID},
		bson.M{"$set": bson.M{"reminder_time": time}},
	)
	if err != nil {
		return err
	}

	return nil
}

func leftUser(bot *tgbotapi.BotAPI, leftUser *tgbotapi.User) error {
	var chatConfigWithUser tgbotapi.ChatConfigWithUser
	chatConfigWithUser.ChatID = nativeGroupChatID
	chatConfigWithUser.UserID = leftUser.ID

	chatMember, err := bot.GetChatMember(chatConfigWithUser)
	if err != nil {
		log.Panic(err)
	}

	_, err = usersCollection.UpdateOne(
		context.TODO(),
		bson.M{"user.id": &leftUser.ID},
		bson.M{"$set": bson.M{"native_chat_member": chatMember}},
	)

	if err != nil {
		return err
	}

	return nil
}

func addDictionary(csvDictionaryPath string, user *tgbotapi.User) error {
	dictionary := loadDictionary(csvDictionaryPath)

	id, err := libraryCollection.InsertOne(
		context.TODO(),
		dictionary,
	)
	if err != nil {
		return err
	}

	_, err = libraryCollection.UpdateOne(
		context.TODO(),
		bson.M{"_id": id.InsertedID},
		bson.M{"$set": bson.D{{Key: "ownerId", Value: user.ID}, {Key: "ownerUsername", Value: user.UserName}}},
	)
	if err != nil {

		return err
	}

	return nil
}

func updateDefaultLibrary(defaultLibraryDirPath string, user *tgbotapi.User) error {
	_, err := libraryCollection.DeleteMany(
		context.TODO(),
		bson.M{"ownerId": user.ID},
	)
	if err != nil {
		return err
	}

	csvDictionariesPathes, err := os.ReadDir(defaultLibraryDirPath)
	if err != nil {
		return err
	}

	for _, csvDictionaryPath := range csvDictionariesPathes {
		addDictionary(defaultLibraryDirPath+"/"+csvDictionaryPath.Name(), user)
	}

	return nil
}

type Dictionary struct {
	ID            primitive.ObjectID `bson:"_id"`
	FilePath      string             `bson:"filePath"`
	FactSet       FactSet            `bson:"factSet"`
	OwnerUsername string             `bson:"ownerUsername"`
	OwnerID       int                `bson:"ownerId"`
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

func updateFactsInBase(user *tgbotapi.User, factSet *FactSet) error {
	for _, fact := range *factSet {

		_, err := libraryCollection.UpdateOne(
			context.TODO(),
			bson.M{"ownerId": user.ID, "factSet.question": fact.Question},
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
		bson.M{"ownerId": user.ID}).Decode(&dictionary); err != nil {

		log.Panic(err)
		return nil, err
	}

	return dictionary.FactSet, err
}

func dumpFactsToBase(user *tgbotapi.User, factSet *FactSet) error {

	var dictionary Dictionary
	//dictionary.FilePath = ""
	dictionary.FactSet = *factSet
	dictionary.OwnerUsername = user.UserName
	dictionary.OwnerID = user.ID

	_, err := libraryCollection.InsertOne(
		context.TODO(),
		dictionary,
	)
	if err != nil {
		return err
	}
	return nil
}

func toSupermemoFactSet(factSet *FactSet) *supermemo.FactSet {

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

func toFactSet(smFactSet *supermemo.FactSet) FactSet {

	var factSet FactSet

	for _, smFact := range *smFactSet {

		q, a, ef, n, interval, intervalFrom := smFact.Dump()
		fmt.Printf("q, a, ef, n, interval, intervalFrom: %v, %v, %v, %v, %v, %v", q, a, ef, n, interval, intervalFrom)

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

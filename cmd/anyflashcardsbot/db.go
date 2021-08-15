package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	//_ "github.com/lib/pq"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

/*
var (
	host     = os.Getenv("DB_SERVER")
	port     = os.Getenv("DB_PORT")
	user     = os.Getenv("POSTGRES_USER")
	password = os.Getenv("POSTGRES_PASSWORD")
	dbname   = os.Getenv("POSTGRES_DB")
	sslmode  = os.Getenv("SSLMODE")
)

var dbInfo = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s", host, port, user, password, dbname, sslmode)

// Insert vocabulary map into base table
func insertVocabMapToTable(vocabulary map[string]string, tablename string) error {
	db, err := sql.Open("postgres", dbInfo)
	if err != nil {
		return fmt.Errorf("cannot open postgres database: %v", err)
	}

	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("cannot start transaction: %v", err)
	}

	defer tx.Commit()

	// Create user vocabulary table if not exist
	_, err = tx.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id SERIAL PRIMARY KEY, word TEXT, meaning TEXT)", tablename))
	if err != nil {
		return err
	}

	// Inset map to table
	for word, meaning := range vocabulary {
		_, err = tx.Exec(fmt.Sprintf("INSERT INTO %s (\"word\", \"meaning\") VALUES ('%s', '%s')", tablename, word, meaning))
		if err != nil {
			return err
		}
	}

	return nil
}*/

/*
// Create table in base
func createTable(tablename string) error {
	db, err := sql.Open("postgres", dbInfo)
	if err != nil {
		return fmt.Errorf("cannot open postgres database: %v", err)
	}

	defer db.Close()

	_, err = db.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s", tablename))
	if err != nil {
		return err
	}

	return nil
}
*/
/*
// Drop table from base
func dropTable(tablename string) error {
	db, err := sql.Open("postgres", dbInfo)
	if err != nil {
		return fmt.Errorf("cannot open postgres database: %v", err)
	}

	defer db.Close()

	_, err = db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", tablename))
	if err != nil {
		return err
	}

	return nil
}
*/
/*
// Prepare base for using
func prepareBase() error {
	db, err := sql.Open("postgres", dbInfo)
	if err != nil {
		return fmt.Errorf("cannot open postgres database: %v", err)
	}

	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("cannot start transaction: %v", err)
	}

	defer tx.Commit()

	// Create table user
	_, err = tx.Exec("CREATE TABLE IF NOT EXISTS student (id INTEGER PRIMARY KEY, name TEXT, vocabulary TEXT)")
	if err != nil {
		return err
	}

	// Create table security
	_, err = tx.Exec("CREATE TABLE IF NOT EXISTS security (id INTEGER PRIMARY KEY, name TEXT, student BOOLEAN, spamer BOOLEAN, pentester BOOLEAN)")
	if err != nil {
		return err
	}

	return nil
}
*/

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

func addNewUsers(bot *tgbotapi.BotAPI, newUsers *[]tgbotapi.User) error {

	for _, newUser := range *newUsers {
		addNewUser(bot, &newUser)
	}

	return nil
}

func addNewUser(bot *tgbotapi.BotAPI, newUser *tgbotapi.User) error {

	var user User
	user.ID = primitive.NewObjectID()
	user.User = *newUser
	user.NativeChatMember.Status = "member"
	user.Dictionary = defaultDictionary

	var chatConfigWithUser tgbotapi.ChatConfigWithUser
	chatConfigWithUser.ChatID = nativeGroupChatID
	chatConfigWithUser.UserID = newUser.ID

	chatMember, err := bot.GetChatMember(chatConfigWithUser)
	if err != nil {
		log.Panic(err)
	}

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

func addDictionary(csvDictionaryPath string, owner string) error {

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
		bson.M{"$set": bson.M{"owner": owner}},
	)
	if err != nil {
		return err
	}

	return nil
}

func updateDefaultLibrary(defaultLibraryDirPath string) error {

	_, err := libraryCollection.DeleteMany(
		context.TODO(),
		bson.M{"owner": "anyflashcardsbot"},
	)
	if err != nil {
		return err
	}

	csvDictionariesPathes, err := os.ReadDir(defaultLibraryDirPath)
	if err != nil {
		return err
	}

	for _, csvDictionaryPath := range csvDictionariesPathes {

		addDictionary(defaultLibraryDirPath+"/"+csvDictionaryPath.Name(), "anyflashcardsbot")

	}

	return nil

}

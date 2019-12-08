package main

import (
	"database/sql"
	"flag"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jedisct1/dlog"
	"gopkg.in/ini.v1"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var DEBUG = false

var RESPONSE_DIR string
var SENDMAIL_BIN string

var config *ini.File

// Function using fmt.Printf for debug printing, but only if DEBUG is true
func DebugFmtPrintf(format string, v ...interface{}) {
	if DEBUG {
		fmt.Printf("DEBUG: "+format, v...)
	}
}
func DebugSyslogFmt(format string, v ...interface{}) {
	if DEBUG {
		dlog.Debug(fmt.Sprintf("DEBUG: "+format, v...))
	}
}

// Return true if file exists and is regular file
func isRegularFile(name string) bool {
	st, err := os.Lstat(name)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}

	return true
}

// Send mail from address to address with given mail content being passed as function pointer
func sendMail(from, to string, populateStdin func(io.WriteCloser)) error {
	cmd := exec.Command(SENDMAIL_BIN, "-i", "-f", from, to)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	err = cmd.Start()
	if err != nil {
		return err
	}
	go func() {
		populateStdin(stdin)
	}()
	err = cmd.Wait()
	if err != nil {
		return err
	}

	return nil
}

type responseSQL struct {
	response string
}

func getDB() (*sql.DB, error) {
	db, err := sql.Open(
		"mysql",
		fmt.Sprintf(
			"%s:%s@tcp(%s:%s)/%s",
			config.Section("mysql").Key("username").String(),
			config.Section("mysql").Key("password").String(),
			config.Section("mysql").Key("host").String(),
			config.Section("mysql").Key("port").String(),
			config.Section("mysql").Key("database").String()))
	return db, err
}

func getResponseMYSQL(recipient *string) (string, error) {
	db, err := getDB()

	if err != nil {
		return "", err
	}

	components := strings.Split(*recipient, "@")
	username, domain := components[0], components[1]

	query := config.Section("mysql").Key("query").String()
	query = strings.Replace(query, "%u", username, -1)
	query = strings.Replace(query, "%d", domain, -1)
	query = strings.Replace(query, "%t", strconv.FormatInt(time.Now().UTC().Unix(), 10), -1)

	DebugSyslogFmt(query)

	row := db.QueryRow(query)

	var key responseSQL
	err = row.Scan(&key.response)

	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no entries for user %s at domain %s", username, domain)
	}

	if err != nil {
		return "", err
	}

	err = db.Close()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`From: %v
To: THIS GETS REPLACED
Content-Type: text/plain; charset=UTF-8
Subject: %v
X-Version: %v
X-Service: %v

%v`, *recipient, config.Section("").Key("query"), config.Section("").Key("version"), config.Section("").Key("service_name"), key.response), nil
}

// Forward email using supplied arguments and stdin (email body)
func forwardEmailAndAutoresponse(recipient string, sender string, responseRate uint) error {
	recipientRateLog := filepath.Join(RESPONSE_DIR, recipient)
	recipientSenderRateLog := filepath.Join(RESPONSE_DIR, recipient, sender)

	response, err := getResponseMYSQL(&sender)
	if err == nil {
		// Check rate log
		sendResponse := true
		if isRegularFile(recipientSenderRateLog) {
			curTime := time.Now()
			st, err := os.Stat(recipientSenderRateLog)
			if err != nil {
				return err
			}
			modTime := st.ModTime()

			if int64(curTime.Sub(modTime))/int64(time.Second) < int64(responseRate) {
				sendResponse = false
				dlog.Info(fmt.Sprintf("Autoresponse has already been sent from %v to %v within last %v seconds",
					recipient, sender, responseRate))
			}
		}

		// If sendResponse is true and sender and recipiend differ, then send response and touch rate log file
		if sendResponse && strings.ToLower(recipient) != strings.ToLower(sender) {
			dlog.Info("Sending Response")
			response = strings.Replace(response, "To: THIS GETS REPLACED", fmt.Sprintf("To: %v", recipient), -1)

			DebugFmtPrintf(response)

			sendMail(recipient, sender, func(sink io.WriteCloser) {
				defer sink.Close()

				io.Copy(sink, os.Stdin)
			})
			err = os.MkdirAll(recipientRateLog, 0770)
			if err != nil {
				return err
			}
			// Touch rate log file
			fl, err := os.Create(recipientSenderRateLog)
			if err != nil {
				return err
			}
			fl.Close()
			dlog.Info(fmt.Sprintf("Autoresponse sent from %v to %v", recipient, sender))
		}
	} else {
		DebugSyslogFmt(fmt.Sprintf("No response found for %v", sender))
	}

	// Now resend original mail
	sendMail(sender, recipient, func(sink io.WriteCloser) {
		defer sink.Close()

		io.Copy(sink, os.Stdin)
	})

	return nil
}

func main() {
	// Connect to syslog
	var err error
	dlog.Init("autoresponder", dlog.SeverityNotice, "DAEMON")

	// Parse command line arguments
	recipientPtr := flag.String("r", "", "Recipient e-mail")
	senderPtr := flag.String("s", "", "Sender e-mail")
	responseRatePtr := flag.Uint("t", 18000, "Response rate in seconds (0 - send each time)")
	showVersion := flag.Bool("V", false, "Show version and exit")
	configPath := flag.String("c", "./config.ini", "Show version and exit")
	testDB := flag.Bool("D", false, "Test database")
	flag.Parse()

	cfg, err := ini.Load(*configPath)
	if err != nil {
		fmt.Printf("Fail to read file: %v", err)
		os.Exit(1)
	}
	config = cfg

	DEBUG, err = config.Section("").Key("debug").Bool()
	RESPONSE_DIR = config.Section("path").Key("response_dir").String()
	SENDMAIL_BIN = config.Section("path").Key("sendmail_bin").String()

	if DEBUG {
		dlog.SetLogLevel(dlog.SeverityDebug)
	}

	if *testDB {
		fmt.Printf("Trying to connect to database...\n")
		db, err := getDB()
		fmt.Printf("Opening connection...\n")
		if err != nil {
			fmt.Printf(err.Error())
			os.Exit(1)
		}
		defer db.Close()
		fmt.Printf("Connected to database!\n")
		os.Exit(0)
	}

	if *showVersion {
		fmt.Printf("autoresponder %v, written by Gurkengewuerz <niklas@mc8051.de> 2019\n", config.Section("").Key("version"))
		os.Exit(0)
	}

	DebugSyslogFmt("Flags:   Recipient: %v, Sender: %v, Response rate: %v",
		*recipientPtr,
		*senderPtr,
		*responseRatePtr)

	if *recipientPtr == "" || *senderPtr == "" {
		fmt.Printf("recipient and/or sender is empty\n")
		DebugSyslogFmt("recipient and/or sender is empty")
		os.Exit(1)
	}

	// Little more validation of recipient and sender
	// Remove path ('/') from both recipient and sender
	*recipientPtr = strings.Replace(*recipientPtr, "/", "", -1)
	*senderPtr = strings.Replace(*senderPtr, "/", "", -1)

	dlog.Info(fmt.Sprintf("Requested email forward from %v, to %v", *senderPtr, *recipientPtr))

	err = forwardEmailAndAutoresponse(*recipientPtr, *senderPtr, *responseRatePtr)
	if err != nil {
		dlog.Error(err.Error())
		os.Exit(1)
	}
}

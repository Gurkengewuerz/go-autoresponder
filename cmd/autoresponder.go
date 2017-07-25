package main

// This is postfix autoresponder, which is rewrite of the autoresponder bash script V1.6.3, written by Charles Hamilton - musashi@nefaria.com
//
// How to make it work on a server with postfix installed:
// =======================================================
// Create autoresponse username:
// useradd -d /var/spool/autoresponse -s `which nologin` autoresponse
//
// Copy autoresponse binary to /usr/local/sbin
// cp autoresponse /usr/local/sbin/
//
// RESPONSE_DIR, RATE_LOG_DIR must be created:
// mkdir -p /var/spool/autoresponse/log /var/spool/autoresponse/responses
// chown -R autoresponse:autoresponse /var/spool/autoresponse
// chmod -R 0770 /var/spool/autoresponse
//
// Edit /etc/postfix/master.cf:
// Replace line:
// smtp inet n - - - - smtpd
// with these two lines (second must begin with at least one space or tab):
// smtp inet n - - - - smtpd
//   -o content_filter=autoresponder:dummy
// At the end of file append the following two lines:
// autoresponder unix - n n - - pipe
//   flags=Fq user=autoresponse argv=/usr/local/sbin/autoresponse -s ${sender} -r ${recipient} -S ${sasl_username} -C ${client_address}
//
//
// Written by Uros Juvan <asmpro@gmail.com> 2017

import (
    "fmt"
    "flag"
    "os"
    "strings"
    "log/syslog"
)

const VERSION = "1.0.0001"
const DEBUG = true

const RESPONSE_DIR = "/var/spool/autoresponse/responses"
const RATE_LOG_DIR = "/var/spool/autoresponse/log"
const SENDMAIL_BIN = "/usr/sbin/sendmail"


// Function using fmt.Printf for debug printing, but only if DEBUG is true
func DebugFmtPrintf(format string, v ...interface{})  {
        if DEBUG {
                fmt.Printf("DEBUG: " + format, v...)
        }
}
func DebugSyslogFmt(syslg *syslog.Writer, format string, v ...interface{})  {
        if DEBUG {
                syslg.Debug(fmt.Sprintf("DEBUG: " + format, v...))
        }
}

// Set autoresponse using supplied arguments and stdin (email body)
func setAutoresponseViaEmail(recipient, sender, saslUser, clientIp string) error {
    //!!!

    return nil
}

func main() {
    // Connect to syslog
    syslg, err := syslog.New(syslog.LOG_MAIL, "autoresponder")
    if err != nil {
        os.Exit(1)
    }
    defer syslg.Close()

    // Parse command line arguments
    recipientPtr := flag.String("r", "", "Recipient e-mail")
    senderPtr := flag.String("s", "", "Sender e-mail")
    saslUserPtr := flag.String("S", "", "SASL authenticated username")
    clientIpPtr := flag.String("C", "", "Client IP address")
    enableAutoResponsePtr := flag.String("e", "", "Enable autoresponse")
    disableAutoResponsePtr := flag.String("d", "", "Disable autoresponse")
    enableExAutoResponsePtr := flag.String("E", "", "Enable existing autoresponse")
    deleteAutoResponsePtr := flag.String("D", "", "Delete autoresponse")
    instructions := flag.Bool("i", false, "Setup instructions")
    flag.Parse()

    DebugSyslogFmt(syslg, "Flags:   Recipient: %v, Sender: %v, SASL authenticated username: %v, Client IP: %v, Enable autoresponse: %v, Disable autoresponse: %v, Enable existing autoresponse: %v, Delete autoresponse: %v, Setup instructions: %v",
        *recipientPtr,
        *senderPtr,
        *saslUserPtr,
        *clientIpPtr,
        *enableAutoResponsePtr,
        *disableAutoResponsePtr,
        *enableExAutoResponsePtr,
        *deleteAutoResponsePtr,
        *instructions)

    // If setup instructions are requested, just print them to stdout and exit
    if *instructions {
        fmt.Print(`
 How to make it work on a server with postfix installed:
 =======================================================
 Create autoresponse username:
 useradd -d /var/spool/autoresponse -s $(which nologin) autoresponse

 Copy autoresponse binary to /usr/local/sbin
 cp autoresponse /usr/local/sbin/

 RESPONSE_DIR, RATE_LOG_DIR must be created:
 mkdir -p /var/spool/autoresponse/log /var/spool/autoresponse/responses
 chown -R autoresponse:autoresponse /var/spool/autoresponse
 chmod -R 0770 /var/spool/autoresponse

 Edit /etc/postfix/master.cf:
 Replace line:
 smtp inet n - - - - smtpd
 with these two lines (second must begin with at least one space or tab):
 smtp inet n - - - - smtpd
   -o content_filter=autoresponder:dummy
 At the end of file append the following two lines:
 autoresponder unix - n n - - pipe
   flags=Fq user=autoresponse argv=/usr/local/sbin/autoresponse -s ${sender} -r ${recipient} -S ${sasl_username} -C ${client_address}
`)
        os.Exit(0)
    }

    // Do some logic on command line arguments
    // Mode 
    // There are two different modes of operation:
    //	mode=0 represents the actions that can not be executed from the command line
    //	mode=1 represents the actions that can be executed from the command line
    mode := 0
    sendResponse := false
    authenticated := false
    if *recipientPtr != "" && *senderPtr != "" {
        sendResponse = true
    }
    if *saslUserPtr != "" {
        authenticated = true
    }
    if *enableAutoResponsePtr != "" || *disableAutoResponsePtr != "" || *enableExAutoResponsePtr != "" || *deleteAutoResponsePtr != "" {
        mode = 1
    }
    DebugSyslogFmt(syslg, "mode=%v, sendResponse=%v, authenticated=%v\n", mode, sendResponse, authenticated)

    recipientParts := strings.Split(*recipientPtr, "@")
    senderParts := strings.Split(*senderPtr, "@")
    // Little more validation of recipient and sender
    if len(recipientParts) < 2 {
        syslg.Err(fmt.Sprintf("Invalid recipient %v", *recipientPtr))
        os.Exit(1)
    }
    if len(senderParts) < 2 {
        syslg.Err(fmt.Sprintf("Invalid sender %v", *senderPtr))
        os.Exit(1)
    }

    // And now descision making
    DebugSyslogFmt(syslg, "recipientUser=%v =? senderUser=%v\n", recipientParts[0], senderParts[0] + "+autoresponse")
    if mode == 0 && authenticated && recipientParts[0] == senderParts[0] + "+autoresponse" {
        syslg.Info(fmt.Sprintf("Requested autoresponse (un)set via email for email %v", *senderPtr))
        err := setAutoresponseViaEmail(*recipientPtr, *senderPtr, *saslUserPtr, *clientIpPtr)
        if err != nil {
            syslg.Err(err.Error())
            os.Exit(1)
        }
    }
    //!!!
}

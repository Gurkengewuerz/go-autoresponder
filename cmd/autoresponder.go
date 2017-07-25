package main

// This is postfix autoresponder, which is rewrite of the autoresponder bash script V1.6.3, written by Charles Hamilton - musashi@nefaria.com
//
// Written by Uros Juvan <asmpro@gmail.com> 2017

import (
    "fmt"
    "flag"
    "os"
)

const VERSION = "1.0.0001"
const DEBUG = true

func DebugFmtPrintf(format string, v ...interface{})  {
        if DEBUG {
                fmt.Printf("DEBUG: " + format, v...)
        }
}

func main() {
    // Parse command line arguments
    recipientPtr := flag.String("r", "", "Recipient e-mail")
    senderPtr := flag.String("s", "", "Sender e-mail")
    saslUserPtr := flag.String("S", "", "SASL authenticated username")
    clientIpPtr := flag.String("C", "", "Client IP address")
    enableAutoResponsePtr := flag.String("e", "", "Enable autoresponse")
    disableAutoResponsePtr := flag.String("d", "", "Disable autoresponse")
    enableExAutoResponsePtr := flag.String("E", "", "Enable existing autoresponse")
    deleteAutoResponsePtr := flag.String("D", "", "Delete autoresponse")
    flag.Parse()

    if DEBUG {
        DebugFmtPrintf(`Flags:
  Recipient: %v
  Sender: %v
  SASL authenticated username: %v
  Client IP: %v
  Enable autoresponse: %v
  Disable autoresponse: %v
  Enable existing autoresponse: %v
  Delete autoresponse: %v
`,
            *recipientPtr,
            *senderPtr,
            *saslUserPtr,
            *clientIpPtr,
            *enableAutoResponsePtr,
            *disableAutoResponsePtr,
            *enableExAutoResponsePtr,
            *deleteAutoResponsePtr)
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
    if *saslUserPtr != "" && os.Getenv("SASL_USERNAME") != "" {
        authenticated = true
    }
    if *enableAutoResponsePtr != "" || *disableAutoResponsePtr != "" || *enableExAutoResponsePtr != "" || *deleteAutoResponsePtr != "" {
        mode = 1
    }
    DebugFmtPrintf("mode=%v, sendResponse=%v, authenticated=%v\n", mode, sendResponse, authenticated)
}

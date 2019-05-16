package main

// This is postfix autoresponder, which is rewrite of the autoresponder bash script V1.6.3, written by Charles Hamilton - musashi@nefaria.com
//
// How to make it work on a server with postfix installed:
// =======================================================
// Create autoresponder username:
// useradd -d /var/spool/autoresponder -s $(which nologin) autoresponder
//
// Copy autoresponder binary to /usr/local/sbin
// cp autoresponder /usr/local/sbin/
// chown autoresponder:autoresponder /usr/local/sbin/autoresponder
// chmod 6755 /usr/local/sbin/autoresponder
//
// RESPONSE_DIR, RATE_LOG_DIR must be created:
// mkdir -p /var/spool/autoresponder/log /var/spool/autoresponder/responses
// chown -R autoresponder:autoresponder /var/spool/autoresponder
// chmod -R 0770 /var/spool/autoresponder
//
// Edit /etc/postfix/master.cf:
// Replace line:
// smtp inet n - - - - smtpd
// with these two lines (second must begin with at least one space or tab):
// smtp inet n - - - - smtpd
//   -o content_filter=autoresponder:dummy
// At the end of file append the following two lines:
// autoresponder unix - n n - - pipe
//   flags=Fq user=autoresponder argv=/usr/local/sbin/autoresponder -s ${sender} -r ${recipient} -S ${sasl_username} -C ${client_address}
//
// Set additional postfix parameter:
// postconf -e 'autoresponder_destination_recipient_limit = 1'
// service postfix restart
//
//
// Written by Uros Juvan <asmpro@gmail.com> 2017

import (
    "fmt"
    "flag"
    "time"
    "os"
    "os/exec"
    "io"
    "io/ioutil"
    "bufio"
    "path/filepath"
    "strings"
    "log/syslog"
)

const VERSION = "1.0.0008"
const DEBUG = true

const RESPONSE_DIR = "/var/spool/autoresponder/responses"
const RATE_LOG_DIR = "/var/spool/autoresponder/log"
const SENDMAIL_BIN = "/usr/sbin/sendmail"


var syslg *syslog.Writer = nil


// Function using fmt.Printf for debug printing, but only if DEBUG is true
func DebugFmtPrintf(format string, v ...interface{})  {
        if DEBUG {
                fmt.Printf("DEBUG: " + format, v...)
        }
}
func DebugSyslogFmt(format string, v ...interface{})  {
        if syslg == nil {
            return
        }
        if DEBUG {
                syslg.Debug(fmt.Sprintf("DEBUG: " + format, v...))
        }
}

// Return true if file exists and is regular file
func isRegularFile(name string) bool {
    st, err := os.Lstat(name)
    if err != nil || ! st.Mode().IsRegular() {
        return false
    }

    return true
}

// Return true if dir exists
func isDir(name string) bool {
    st, err := os.Lstat(name)
    if err != nil || ! st.Mode().IsDir() {
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

// Set autoresponse using supplied arguments and stdin (email body)
func setAutoresponseViaEmail(recipient, sender, saslUser, clientIp string) error {
    senderResponsePath := filepath.Join(RESPONSE_DIR, sender)
    if isRegularFile(senderResponsePath) {
        err := deleteAutoresponse(sender, true)
        if err != nil {
            return err
        }

        if ! isRegularFile(senderResponsePath) {
            syslg.Info(fmt.Sprintf("Autoresponse disabled for address %v by SASL authenticated user: %v from: %v",
                sender, saslUser, clientIp))
            // Send mail via sendmail
            sendMail(recipient, sender, func(sink io.WriteCloser) {
                defer sink.Close()

                sink.Write([]byte(fmt.Sprintf("From: %v\nTo: %v\nSubject: Autoresponder\n\n"+
                    "Autoresponse disabled for %v by SASL authenticated user: %v from: %v\n",
                    recipient, sender, sender, saslUser, clientIp)))
            })
        } else {
            return fmt.Errorf("Autoresponse could not be disabled for address %v", sender)
        }
    } else {
        // Cat stdin to response file for the user, removing unneeded headers in the process
        // Only From:, To: and Subject: are needed.
        // To: ... is replaced with To: THIS GETS REPLACED
        // Subject: ... is replaced with Subject: Autoresponder
        fl, err := os.OpenFile(senderResponsePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
        if err != nil {
            return fmt.Errorf("Autoresponse could not be enabled for address %v: %v", sender, err)
        }
        fl.Chmod(0660)
        reader := bufio.NewReader(os.Stdin)
        writer := bufio.NewWriter(fl)
        defer func() {
            writer.Flush()
            fl.Close()
        }()
        state := 0
        for {
            line, err := reader.ReadString('\n')
            if err != nil {
                if err == io.EOF {
                    break
                }
                return err
            }
            //fmt.Printf("Read line: '%v'\n", line)

            switch state {
            // state 0 (mail header)
            case 0:
                if line == "\n" || line == "\r\n" || line == "\r" {
                    _, err = writer.WriteString(line)
                    state = 1
                    continue
                }

                switch true {
                case strings.Index(strings.ToLower(line), "from: ") == 0 ||
                    strings.Index(strings.ToLower(line), "content-type: ") == 0 ||
                    strings.Index(strings.ToLower(line), "content-transfer-encoding: ") == 0 ||
                    strings.Index(strings.ToLower(line), "mime-version: ") == 0:
                    _, err = writer.WriteString(line)

                case strings.Index(strings.ToLower(line), "to: ") == 0:
                    _, err = writer.WriteString("To: THIS GETS REPLACED\n")

                case strings.Index(strings.ToLower(line), "subject: ") == 0:
                    _, err = writer.WriteString("Subject: Autoresponder\n")
                }

            case 1:
                _, err = writer.WriteString(line)
            }

            if err != nil {
                return err
            }
        }

        if isRegularFile(senderResponsePath) {
            syslg.Info(fmt.Sprintf("Autoresponse enabled for address %v by SASL authenticated user: %v from: %v",
                sender, saslUser, clientIp))
            // Send mail via sendmail
            sendMail(recipient, sender, func(sink io.WriteCloser) {
                defer sink.Close()
                sink.Write([]byte(fmt.Sprintf("From: %v\nTo: %v\nSubject: Autoresponder\n\n"+
                    "Autoresponse enabled for %v by SASL authenticated user: %v from: %v\n",
                    recipient, sender, sender, saslUser, clientIp)))
            })
        } else {
            return fmt.Errorf("Autoresponse could not be enabled for address %v", sender)
        }
    }

    return nil
}

// Forward email using supplied arguments and stdin (email body)
func forwardEmailAndAutoresponse(recipient, sender, saslUser, clientIp string, responseRate uint) error {
    recipientResponsePath := filepath.Join(RESPONSE_DIR, recipient)
    recipientRateLog := filepath.Join(RATE_LOG_DIR, recipient)
    recipientSenderRateLog := filepath.Join(RATE_LOG_DIR, recipient, sender)

    if isRegularFile(recipientResponsePath) {
        // Check rate log
        sendResponse := true
        if isRegularFile(recipientSenderRateLog) {
            curTime := time.Now()
            st, err := os.Stat(recipientSenderRateLog)
            if err != nil {
                return err
            }
            modTime := st.ModTime()

            if int64(curTime.Sub(modTime)) / int64(time.Second) < int64(responseRate) {
                sendResponse = false
                syslg.Info(fmt.Sprintf("Autoresponse has already been sent from %v to %v within last %v seconds",
                    recipient, sender, responseRate))
            }
        }

        // If sendResponse is true and sender and recipiend differ, then send response and touch rate log file
        if sendResponse && strings.ToLower(recipient) != strings.ToLower(sender) {
            //fmt.Println("Sending response")
            fl, err := os.Open(recipientResponsePath)
            if err != nil {
                return err
            }
            defer fl.Close()
            sendMail(recipient, sender, func(sink io.WriteCloser) {
                defer sink.Close()

                // Open recipientResponsePath file and do some replacements on the fly
                reader := bufio.NewReader(fl)
                state := 0
                for {
                    line, err := reader.ReadString('\n')
                    if err != nil {
                        if err == io.EOF {
                            break
                        }
                        syslg.Err(err.Error())
                        return
                    }

                    switch state {
                    case 0:
                        switch true {
                        case line == "\n" || line == "\r\n" || line == "\r":
                            state = 1

                        case strings.Index(strings.ToLower(line), "from: ") == 0:
                            line = fmt.Sprintf("From: %v\n", recipient)

                        case strings.Index(strings.ToLower(line), "to: ") == 0:
                            line = fmt.Sprintf("To: %v\n", sender)
                        }
                        fallthrough

                    default:
                        sink.Write([]byte(line))
                    }
                }
            })
            err = os.MkdirAll(recipientRateLog, 0770)
            if err != nil {
                return err
            }
            // Touch rate log file
            fl, err = os.Create(recipientSenderRateLog)
            if err != nil {
                return err
            }
            fl.Close()
            syslg.Info(fmt.Sprintf("Autoresponse sent from %v to %v", recipient, sender))
        }
    }

    // Now resend original mail
    sendMail(sender, recipient, func(sink io.WriteCloser) {
        defer sink.Close()

        io.Copy(sink, os.Stdin)
    })

    return nil
}

// Get text editor
func getTextEditor() string {
    editor := os.Getenv("EDITOR")
    if editor == "" {
        editor = "vi"
    }

    return editor
}

// Get file modification time
func getFileModTime(name string) (t time.Time, err error) {
    st, err := os.Stat(name)
    if err != nil {
        return t, err
    }
    t = st.ModTime()

    return
}

// Enable autoresponse for email
func enableAutoresponse(email string) error {
    emailResponsePath := filepath.Join(RESPONSE_DIR, email)
    editFilePath := emailResponsePath

    // If editFilePath does not exist, also try to enable previosly disabled autoresponse
    if ! isRegularFile(editFilePath) {
        enableExAutoresponse(email, true)
    }

    // If file does not exist yet, create template file as tmp file
    var oldModTime, newModTime time.Time
    if ! isRegularFile(editFilePath) {
        editFile, err := ioutil.TempFile("", "autoresponder")
        if err != nil {
            return err
        }
        editFilePath = editFile.Name()
        defer os.Remove(editFilePath)

        writer := bufio.NewWriter(editFile)

        // Write template to file
        writer.WriteString(fmt.Sprintf(`From: %v
To: THIS GETS REPLACED
Content-Type: text/plain; charset=UTF-8
Subject: Autoresponder

mail body`, email))

        writer.Flush()
        editFile.Close()
    }
    oldModTime, err := getFileModTime(editFilePath)
    if err != nil {
        return err
    }

    // Invoke either EDITOR environment or vi command
    cmd := exec.Command(getTextEditor(), editFilePath)
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    err = cmd.Run()
    if err != nil {
        return err
    }

    newModTime, err = getFileModTime(editFilePath)
    if err != nil {
        return err
    }

    if oldModTime != newModTime {
        if emailResponsePath != editFilePath {
            // Open editFilePath for reading and emailResponsePath for writing and Copy content over
            tmpFl, err := os.Open(editFilePath)
            if err != nil {
                return err
            }
            defer tmpFl.Close()

            resFl, err := os.OpenFile(emailResponsePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0660)
            if err != nil {
                return err
            }
            resFl.Chmod(0660)
            defer resFl.Close()

            _, err = io.Copy(resFl, tmpFl)
            if err != nil {
                return err
            }
        }
        msg := fmt.Sprintf("Edited %v", emailResponsePath)
        syslg.Info(msg)
        fmt.Println(msg)
    } else {
        msg := fmt.Sprintf("Editing %v aborted!", emailResponsePath)
        fmt.Println(msg)
        return fmt.Errorf("%v", msg)
    }

    return nil
}

// Disable autoresponse for email
func disableAutoresponse(email string) error {
    emailResponsePath := filepath.Join(RESPONSE_DIR, email)

    if isRegularFile(emailResponsePath) {
        disableEmailResponsePath := emailResponsePath + "_DISABLED"
        os.Remove(disableEmailResponsePath)
        err := os.Rename(emailResponsePath, disableEmailResponsePath)
        if err != nil {
            return err
        }
        msg := fmt.Sprintf("Disabled %v", emailResponsePath)
        syslg.Info(msg)
        fmt.Println(msg)
    } else {
        msg := fmt.Sprintf("%v does not exist, thus it cannot be disabled!", emailResponsePath)
        fmt.Println(msg)
        return fmt.Errorf("%v", msg)
    }

    return nil
}

// Enable existing autoresponse for email
func enableExAutoresponse(email string, nostdout bool) error {
    emailResponsePath := filepath.Join(RESPONSE_DIR, email)
    disableEmailResponsePath := emailResponsePath + "_DISABLED"

    if isRegularFile(disableEmailResponsePath) {
        os.Remove(emailResponsePath)
        err := os.Rename(disableEmailResponsePath, emailResponsePath)
        if err != nil {
            return err
        }
        msg := fmt.Sprintf("Enabled %v", emailResponsePath)
        syslg.Info(msg)
        fmt.Println(msg)
    } else {
        msg := fmt.Sprintf("%v does not exist, thus it cannot be enabled!", disableEmailResponsePath)
        if ! nostdout {
            fmt.Println(msg)
        }
        return fmt.Errorf("%v", msg)
    }

    return nil
}

// Delete autoresponse for email
func deleteAutoresponse(email string, nostdout bool) error {
    deleteResponsePath := filepath.Join(RESPONSE_DIR, email)
    disabledDeleteResponsePath := deleteResponsePath + "_DISABLED"
    recipientRateLog := filepath.Join(RATE_LOG_DIR, email)

    if isRegularFile(deleteResponsePath) {
        os.Remove(disabledDeleteResponsePath)
        os.RemoveAll(recipientRateLog)
        err := os.Remove(deleteResponsePath)
        if err != nil {
            msg := fmt.Sprintf("%v cannot be deleted: %v", deleteResponsePath, err)
            if ! nostdout {
                fmt.Println(msg)
            }
            return fmt.Errorf("%v", msg)
        }
        msg := fmt.Sprintf("Delete %v done", deleteResponsePath)
        if ! nostdout {
            fmt.Println(msg)
        }
        syslg.Info(msg)
    } else {
        msg := fmt.Sprintf("%v does not exist, thus it cannot be deleted!", deleteResponsePath)
        if ! nostdout {
            fmt.Println(msg)
        }
        return fmt.Errorf("%v", msg)
    }

    return nil
}

func main() {
    // Connect to syslog
    var err error
    syslg, err = syslog.New(syslog.LOG_MAIL, "autoresponder")
    if err != nil {
        fmt.Println(err.Error())
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
    instructionsPtr := flag.Bool("i", false, "Setup instructions")
    responseRatePtr := flag.Uint("t", 86400, "Response rate in seconds (0 - send each time)")
    showVersion := flag.Bool("V", false, "Show version and exit")
    flag.Parse()

    if *showVersion {
        fmt.Printf("autoresponder %v, written by Uros Juvan <asmpro@gmail.com> 2017-2019\n", VERSION)
        os.Exit(0)
    }

    DebugSyslogFmt("Flags:   Recipient: %v, Sender: %v, SASL authenticated username: %v, Client IP: %v, Enable autoresponse: %v, Disable autoresponse: %v, Enable existing autoresponse: %v, Delete autoresponse: %v, Setup instructions: %v, Response rate: %v",
        *recipientPtr,
        *senderPtr,
        *saslUserPtr,
        *clientIpPtr,
        *enableAutoResponsePtr,
        *disableAutoResponsePtr,
        *enableExAutoResponsePtr,
        *deleteAutoResponsePtr,
        *instructionsPtr,
        *responseRatePtr)

    // If setup instructions are requested, just print them to stdout and exit
    if *instructionsPtr {
        fmt.Print(`
 How to make it work on a server with postfix installed:
 =======================================================
 Create autoresponder username:
 useradd -d /var/spool/autoresponder -s $(which nologin) autoresponder

 Copy autoresponder binary to /usr/local/sbin
 cp autoresponder /usr/local/sbin/
 chown autoresponder:autoresponder /usr/local/sbin/autoresponder
 chmod 6755 /usr/local/sbin/autoresponder

 RESPONSE_DIR, RATE_LOG_DIR must be created:
 mkdir -p /var/spool/autoresponder/log /var/spool/autoresponder/responses
 chown -R autoresponder:autoresponder /var/spool/autoresponder
 chmod -R 0770 /var/spool/autoresponder

 Edit /etc/postfix/master.cf:
 Replace line:
 smtp inet n - - - - smtpd
 with these two lines (second must begin with at least one space or tab):
 smtp inet n - - - - smtpd
   -o content_filter=autoresponder:dummy
 At the end of file append the following two lines:
 autoresponder unix - n n - - pipe
   flags=Fq user=autoresponder argv=/usr/local/sbin/autoresponder -s ${sender} -r ${recipient} -S ${sasl_username} -C ${client_address}

 Set additional postfix parameter:
 postconf -e 'autoresponder_destination_recipient_limit = 1'
 service postfix restart
`)
        os.Exit(0)
    }

    // Do some logic on command line arguments
    // Mode 
    // There are two different modes of operation:
    //	mode=0 represents the actions that can not be executed from the command line
    //	mode=1 represents the actions that can be executed from the command line
    mode := 0
    authenticated := false
    if *saslUserPtr != "" {
        authenticated = true
    }
    if *enableAutoResponsePtr != "" || *disableAutoResponsePtr != "" || *enableExAutoResponsePtr != "" || *deleteAutoResponsePtr != "" {
        mode = 1
    }
    DebugSyslogFmt("mode=%v, authenticated=%v\n", mode, authenticated)

    // Little more validation of recipient and sender
    // Remove path ('/') from both recipient and sender
    *recipientPtr = strings.Replace(*recipientPtr, "/", "", -1)
    *senderPtr = strings.Replace(*senderPtr, "/", "", -1)
    recipientParts := strings.Split(*recipientPtr, "@")
    senderParts := strings.Split(*senderPtr, "@")

    // And now descision making
    DebugSyslogFmt("recipientUser=%v =? senderUser=%v\n", recipientParts[0], senderParts[0] + "+autoresponse")
    switch true {
    //   - (un)set autoresponse via email
    case mode == 0 && recipientParts[0] == senderParts[0] + "+autoresponse":
        syslg.Info(fmt.Sprintf("Requested autoresponse (un)set via email for email %v", *senderPtr))

        // Do not allow unauthenticated changes
        if ! authenticated {
            syslg.Warning(fmt.Sprintf("Unauthenticated attempt to set autoresponse message for %v from %v !",
                *senderPtr, *clientIpPtr))
            os.Exit(0)
        }

        err := setAutoresponseViaEmail(*recipientPtr, *senderPtr, *saslUserPtr, *clientIpPtr)
        if err != nil {
            syslg.Err(err.Error())
            os.Exit(1)
        }

    //  - forward mail and either send response if set and enough time has passed
    case mode == 0 && strings.Index(*recipientPtr, "+autoresponse") == -1:
        syslg.Info(fmt.Sprintf("Requested email forward from %v, to %v", *senderPtr, *recipientPtr))

        err := forwardEmailAndAutoresponse(*recipientPtr, *senderPtr, *saslUserPtr, *clientIpPtr, *responseRatePtr)
        if err != nil {
            syslg.Err(err.Error())
            os.Exit(1)
        }

    //  - set autoresponse via cli
    case mode == 1 && *enableAutoResponsePtr != "":
        syslg.Info(fmt.Sprintf("Requested enable autoresponse for %v", *enableAutoResponsePtr))

        err := enableAutoresponse(*enableAutoResponsePtr)
        if err != nil {
            syslg.Err(err.Error())
            os.Exit(1)
        }

    //  - disable autoresponse via cli
    case mode == 1 && *disableAutoResponsePtr != "":
        syslg.Info(fmt.Sprintf("Requested disable autoresponse for %v", *disableAutoResponsePtr))

        err := disableAutoresponse(*disableAutoResponsePtr)
        if err != nil {
            syslg.Err(err.Error())
            os.Exit(1)
        }

    //  - enable existing autoresponse via cli
    case mode == 1 && *enableExAutoResponsePtr != "":
        syslg.Info(fmt.Sprintf("Requested enable existing autoresponse for %v", *enableExAutoResponsePtr))

        err := enableExAutoresponse(*enableExAutoResponsePtr, false)
        if err != nil {
            syslg.Err(err.Error())
            os.Exit(1)
        }

    //  - delete existing autoresponse via cli
    case mode == 1 && *deleteAutoResponsePtr != "":
        syslg.Info(fmt.Sprintf("Requested delete autoresponse for %v", *deleteAutoResponsePtr))

        err := deleteAutoresponse(*deleteAutoResponsePtr, false)
        if err != nil {
            syslg.Err(err.Error())
            os.Exit(1)
        }
    }
}

This is postfix autoresponder is originally written by [Charles Hamilton](mailto:musashi@nefaria.com),
rewritten by [asmpro](https://github.com/asmpro/mailPostfixAutoresponder) and now rewritten by [me](https://gurkengewuerz.de) for my purpose.

## Installation

#### Create autoresponder user

    useradd -d /var/spool/autoresponder -s $(which nologin) autoresponder

#### Compile autoresponder

    go get git.gurkengewuerz.de/Gurkengewuerz/go-autoresponder

### Copy autoresponder binary to /usr/local/sbin

    cp ~/gowork/bin/go-autoresponder /usr/local/sbin/autoresponder
    chown autoresponder:autoresponder /usr/local/sbin/autoresponder
    chmod 6755 /usr/local/sbin/autoresponder

### Create response_dir

    mkdir -p /var/spool/autoresponder/responses
    cp ~/gowork/src/git.gurkengewuerz.de/Gurkengewuerz/go-autoresponder/config.ini.sample /var/spool/autoresponder/config.ini
    chown -R autoresponder:autoresponder /var/spool/autoresponder
    chmod -R 0770 /var/spool/autoresponder

### Create Log Path

    touch /var/log/autoresponder.log
    chown autoresponder:autoresponder /var/log/autoresponder.log

### Edit /etc/postfix/master.cf
Replace line:

    smtp inet n - - - - smtpd

with these two lines (second must begin with at least one space or tab):

    smtp inet n - - - - smtpd
      -o content_filter=autoresponder:dummy

At the end of file append the following two lines:

    autoresponder unix - n n - - pipe
      flags=Fq user=autoresponder argv=/usr/local/sbin/autoresponder -s ${sender} -r ${recipient} -c /var/spool/autoresponder/config.ini -logfile /var/log/autoresponder.log

### Set additional postfix parameter

    postconf -e 'autoresponder_destination_recipient_limit = 1'

### Restart postfix

    service postfix restart

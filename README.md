# DNSHub

DNSHub - Fast and Reliable DNS Relay Hub

DNSHub is a flexible and efficient DNS relay hub designed to enhance DNS query performance and reliability.

- Primary & Backup DNS: Queries are sent to multiple primary DNS servers, and the first successful response is used. If all primary servers fail, backup DNS servers will be used (if fallback is enabled).
- Response Caching: Cache DNS results to speed up future queries.
- Domain Exclusion List: Configure specific domains to exclusively use backup DNS servers, bypassing primary ones.

DNSHub ensures fast resolution with high availability, making it ideal for both personal and enterprise use.

## Installation

```bash
curl -Lo- https://github.com/sunshineplan/dnshub/releases/latest/download/release-linux.tar.gz | tar zxC .
chmod +x dnshub
./dnshub install
./dnshub start
```
You can also build your own binary by:
```cmd
git clone https://github.com/sunshineplan/dnshub.git
cd dnshub
go build
```

## Credits

This repo relies on:

  * [github.com/miekg/dns](https://github.com/miekg/dns)
  * [github.com/fsnotify/fsnotify](https://github.com/fsnotify/fsnotify)

## Usage

### Command Line

```
  -primary <string>
    	List of primary DNS, separated with commas
  -backup <string>
    	List of backup DNS
  -exclude <file>
    	Exclude list file
  -hosts <file>
    	Hosts file
  -proxy <string>
    	List of proxies for DNS
  -port <port>
    	DNS port (default 53)
  -fallback
    	Enable fallback
  -update <url>
    	Update URL
```

### Service Command

```
  install
    	Install service
  uninstall/remove
    	Uninstall service
  run
    	Run service executor
  test
    	Run service test executor	
  start
    	Start service
  stop
    	Stop service
  restart
    	Restart service
  update
    	Update service files if update url is provided
```

## Example config

### config.ini

```
primary  = 1.1.1.1@doh,1.0.0.1@tcp,8.8.4.4
backup   = *8.8.8.8@dot
proxy    = socks5://username:password@localhost:1080
hosts    = /etc/hosts
port     = 53
fallback = true
```

### exclude.list

```
github.com
```

### hosts

```
8.8.8.8 dns.google
```

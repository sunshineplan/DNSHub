# Proxy DNS

Proxy DNS serves as a DNS proxy server.

The domains in remote list will use remote DNS for query. Remote DNS must support tcp connection.
If remote list file is changed, it will be reloaded automatically.
If proxy is specified, it will connect remote DNS using this proxy.
Currently, support http,https,socks5,socks5h proxy.

## Installation

```bash
curl -Lo- https://github.com/sunshineplan/proxydns/releases/latest/download/release-linux.tar.gz | tar zxC .
chmod +x proxydns
./proxydns install
./proxydns start
```
You can also build your own binary by:
```cmd
git clone https://github.com/sunshineplan/proxydns.git
cd proxydns
go build
```

## Credits

This repo relies on:

  * [github.com/miekg/dns](https://github.com/miekg/dns)
  * [github.com/vharitonsky/iniflags](https://github.com/vharitonsky/iniflags)

## Usage

### Command Line

```
  -local <string>
    	List of local DNS servers, separated with commas. Port numbers may also optionally be
		given as :<port-number> after each address
  -remote <string>
    	List of remote DNS servers which must support tcp (default "8.8.8.8")
  -list <file>
    	Remote list file
  -hosts <file>
    	Hosts file
  -proxy <string>
    	Remote DNS proxy, support http,https,socks5,socks5h proxy
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
local    = 1.1.1.1
remote   = 8.8.8.8
list     = remote.list
hosts    = /etc/hosts
proxy    = http://127.0.0.1:1080
port     = 53
fallback = true
```

### remote.list

```
google.com
```

### hosts

```
8.8.8.8 dns.google
```

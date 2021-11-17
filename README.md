# blibee-dnsproxy

## Usage

```
sudo ./blibee-dnsproxy-go-linux-amd64 -f 10.1.0.67:53 -c 10.253.0.201:53 -p 119.29.29.29:53 -l 0.0.0.0:53
```

Issue a request to the server just started:

```
host baidu.com 127.0.0.1
host google.com 127.0.0.1
```

`baidu.com` is dispatched to `114.114.114.114`, but `google.com` is dispatched to `8.8.8.8` because its server is not located in China.

### How does it work?

`blibee-dnsproxy` tries to dispatch the request to a DNS upstream based on the whitedomain, forward to -f and -c upstream server to resolve domain if request domain in whitedomian, else to -p upstream server, when the result arrived, check the -f upstream releated result, if exist, response 

to client, else reponse from -c upstream server. if the request domain not in whitedomian, from -p/public upstream server to get the result and then response to client.


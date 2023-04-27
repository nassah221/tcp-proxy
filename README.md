# Notes

## Proxy Server (Go)

I have chosen to write bigger part of the project in Go because of being more comfortable and fluent in it since I write it daily.

### File Structure

I went with a single package as I didn't want anything complicated. The whole server is a single `main` package broken down into four files.

`config.go` and `app.go` are for reading and parsing the configuration file.

The business logic is contained in `proxy.go`.

`main.go` glues everything together.

### How It Works

The application takes a single flag '`config`' to specify the config path. By default, it uses the `config.json` at the root of this repository if the flag is not specified.

Example usage: `tcp-proxy -config example.json`

#### Breakdown

I am completely transparent when I say I didn't have basic knowledge about the workings of a proxy-server or proxies in general. Doing a lot of research I figured it out as I went through different material on the internet. At first I was over-thinking it in trying to come up with the most robust, best possible implementation in the first-go but in the end went with something really simple.

Go makes it incredibly simple with <sup>[1]</sup> `io.Copy(src, dst)`. With that in mind, I only had to make sense of the concurrency-model of the application as I have found the concurrency in Go to be really welcoming, easy to get get started with but tricky to get right.

From the application point-of-view, it's clear from the requirements to listen and accept the connections as they come in. It spins-up goroutines for <sup>[2]</sup> configured listeners on each  port in the application 'main loop' and blocks. Each listener accepts new client connections and serves them in their separate goroutines by <sup>[3]</sup>dialing the target server and tunneling the connection client <-> proxy <-> target.

The application balances between multiple targets by using round-robin approach. It is ensured that new client connections are successful in case of target server unavailability for multiple targets.

Finally, Ctrl+C signals for shutdown which is made possible with context propagation. The spawned goroutines terminate/return finally unblocking the main loop, exiting the application.

<sup>[1]</sup><sup>[2]</sup><sup>[3]</sup> references to consideration which I go over in the next section

### Considerations

In the following paragraphs I go over some of thoughts and ideas I had for the proxy.

- Any resilient system should be able to withstand and recover from failures. Consider outage of one or multiple targets servers. All of the connections to the affected servers are moved over to target server geographically close in proximity to the affected servers. There are probably regions and zones within each region to make this possible. Now, there are a couple of things that are implicit in this case. First, the proxy is aware of the geographical distribution of the target servers and their respective loads/latencies in realtime. And, it is able to ascertain the best possible target(s) to distribute affected client connections to and executes it seamlessly.
- If the proxy is publicly available and not just through our own client(website, webapp) then it may be better to have filters/rules in place for incoming client traffic. I'm not sure if inspecting the traffic just to determine the nature/authenticity of the request falls into any gray areas legally/ethically.
- Use rate-limiting for the TCP connection to limit overall bandwidth and read per event. This would ensure that one client's heavy resource usage doesn't end up affecting the bandwidth of the other clients. I go into depth on this particular topic in the next section as well as some of the mistakes I made.
- I left out some of the parameters used for configuring the TCP connection such as read/write deadlines, timeouts and keep-alive. These settings should be configured based on the system requirements.
- It should use TLS.
- I only used the std log package for logging which should be replaced with a suitable package like zerolog or zap for proper logging.
- Tracing is a must for any production grade system though I imagine it would be a somewhat complex due to the whole parsing/transforming of raw TCP packets and such. If we actually have HTTP on top the request headers can be injected with trace-ids and context to make it possible.

<sup>[1]</sup> During my [research](https://www.sobyte.net/post/2022-03/golang-zero-copy/) I came across the `net.TCPConn.ReadFrom()` method which is apparently a zero-copy optimization for TCP sockets. Though I avoided using it. As the famous saying goes:

> Premature optimization is the root of all evil

<sup>[2]</sup> In a production application it would make sense for each listener to be resilient to errors/failures i.e. it restarts without affecting the application itself or other connections or have some sort of circuit breaker/backoff logic.

<sup>[3]</sup> It is likely that in a production application a connection pool is used unlike my implementation which dials/opens new connections to the target server as new client connections come in. With a connection pool, connecting clients are immediately hooked up to a connection from the pool. This is efficient as opposed to dialing the target connections for each connecting client.
Additionally, this system would also have health checks for respective servers so it can be resilient and route traffic accordingly in case of target server downtime.

### A Ways to Go

I think the biggest bottleneck will prove to be not rate-limiting the client connections. A client utilizing the connection heavily might starve other clients of network, compute resources. Limiting reads/writes per event might go a long way in preventing this problem.

Secondly, the proxy is currently dumb in the sense that it doesn't know which target server will be suitable for each connecting client. The current round-robin load balancing is naive and a production system should have something more sophisticated.

I would also want to define clear constraints with timeouts, deadlines and keep-alives instead of just using the defaults.

###  Making Customers Happy

I may have cheated a little bit by reading a few of the fly.io blogs (they're great and I enjoyed reading them). I know scale-to-zero has been the most requested feature, so I would implement it to make the customers happy. If the connection is idle for a defined period of time, I'd implement shutting down processes on the target-server so the customers don't incur cost on idle resource. This would probably be a little more complicated than I'm making it sound but the general idea remains the same.

### Starting Over

I only grasp the importance of including this section after I have already implemented both parts of the project. I was so absorbed with the fine implementation details that I made some errors which I might lose points for . The reason for mentioning this is to be transparent about myself. Sometimes, I focus too closely on the details and fail to see the bigger picture. However, I believe that engineering is an iterative process and we wouldn't be here if software turned out perfect in the first go(pun not intended).

So to answer the question
> If you were starting over, is there anything you’d do differently?

Yes I would, here's what.

Starting off, I would need the following numbers:

- Network bandwidth available to the server(s?)/data-centre? that will run/host the proxy
- Throughput/RPS of the target server(s)
- Round-trip time(RTT) of the target server(s)

Then calculate max throughput by the formula:

`Max Throughput (bits per second)`  =  (`Bytes per request` $\times$ `8`) / `RTT`

e.g.

Considering no packet-loss, with a 32KB buffer and an RTT of 200 ms (0.200 s),

Throughput = (`32,000`$\times$ `8`) / `0.200` = 1,280,000 = 1.28 Mbps

This is the maximum threshold I should have for each client. Practically, the bandwidth would be aggregated for read/write operations however for simplicity let's only consider the read from client case. If I am doing HTTP over TCP through my own client/website/dashboard then I might be able to determine the payload size which allows me to calculate the throughput and in turn rate-limit each connection divided over the available network bandwidth. By default, `io.Copy()` has a 32KB buffer window, so the proxy may read into a smaller buffer size based on the calculated throughput and rate-limit the client connections. This allows for a layer of control over how traffic is routed to each target server as I know their load capacity/ throughput and might be able to derive some logic to better manage the process in a controlled manner from proxy to target. I even found [a package](https://pkg.go.dev/github.com/gerritjvv/tcpshaper@v0.0.0-20200213131014-28d4771b71ab/bandwidth) that implements rate-limiting over TCP but it shouldn't be too difficult to roll my own implementation.

From a business perspective, if I am offering a tiered system to my customers, this mentioned approach allows me to prioritize the bandwidth wrt to the customer tier.

### Global Scale

In the previous section, I went over of the factors that might be considered designing a real-world system that scales globally. These factors may not be totally reflective of the real ones, which of course you - the reader would know, but this is my stage so you will have to bear with me for a little longer :)

Knowing the throughput of my target servers allows me to design the proxy around it since now I know the upper bound of that constraint. For working out the maximum number of connections the proxy can handle, I would have to carry-out load testing scenarios and benchmarks to find the bottleneck and max threshold for the number of connections. A clustered version of the proxy would have the ability to scale horizontally automatically so when the loads tends to the max threshold another node is created/spawned and the load is then distributed accordingly i.e. the first node may hand-over half of it's load to the second node or new connections just get handed over to the second node that's newly created. This of course would require a load-balancer in front of the proxy nodes itself. There would ideally be a directory/discovery service in-front of the proxy which would route the traffic as needed as well as managing the creation/shutdown of the proxy nodes as necessary.

## Proxy Tester (Rust)

It took me a little more than expected to be able to come up with something viable in Rust because I hadn't touched Rust in the last ~7 months. It turns out, asynchronous Rust is gnarly especially compared to Go which makes it look effortless and my frankly Rust is a  bit rusty(pun very much intended) to say the least.

The tester parses the same config file as for the proxy. It then makes a single tcp connection to each of the application 'Ports' and sends 'n' number of 'hello world' messages and measures the latency for each message to be echoed back. On completing the send/receive loop, it reports the test metrics. I have kept it simple and only do a single read per each write of the 'hello world' message. The tester takes two positional arguments.

1. to specify the config.json path (default: ./config.json)
2. to specify 'n' number of messages to send per tcp connection (default: 10)

Example usage: `proxy-client config.json 50`

I feel like there are more rough edges with this tool than I'd be comfortable to share for the project, however, I didn't want to spend too much time on it. This is also reflective of my current Rust expertise, however, please know that I absolutely love Rust(and Go as well) and if everything works out, would cherish the opportunity to write it and eventually get better at it.

### Metrics

For the tester I tried to follow the approach that usually load testing tools adopt. My main intention with this was to test the proxy, yes, but to also design something to benchmark the proxy, though it's far from being latter.

The tool prints the following metrics to stdout on completion

1. p50, p90, p99 and p999 response latencies (ms)
2. throughput in requests per second

### Ideas and Limitation

I have some that I wasn't able to implement but would be great to have as features.

Currently, the tester only makes one connection for each application port as found in the config. Perhaps, it would be better this was configuration through a flag and the tester could have open n number of connections per port and perform the load testing.

There's no control over how the messages are sent. May be I want to send n number of messages in a burst or delay each send by any arbitrary period of time(slow client?).

It's too simple to send a 'hello world' message. A good approach would be to have a flag to specify a custom message or to define multiple messages in a configuration file format.

It would be great to have a different traffic load scenarios for the tester i.e. mimic a sine wave, saw-tooth wave or constant load as no. of connections made or messages sent. Additionally, the I could also specify a maximum rps and a step-size to increase the load after n number of seconds.

I should be able to specify either to wait for all the in-flight requests to complete or terminate the test as soon as the termination condition is met. There could also be a metric for reporting no. of dropped messages in this scenario.

It would also be great to have a graph of avg. throughput and avg. nth percentile latencies on y-axis plotted against no. of connections on x-axis.

Have an option to write the test report to a file.

Of course, there's too many fine-grained controls to consider but I think implementing the few I have mentioned may be a good starting point to having a versatile tool.

## Finally

Whatever your verdict may be for the my submission, I absolutely loved working on this project and with this hiring format. I wish more companies would do the same. I learned a ton of new things. Thank you  for this opportunity and for your time in reviewing these notes and my code. I wish me best of luck :D

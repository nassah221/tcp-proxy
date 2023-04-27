use std::net::SocketAddr;
use std::str;
use std::time::Instant;

use histogram::Histogram;
use std::sync::mpsc::{self, Sender};
use tokio::{
    io::{self, AsyncReadExt, AsyncWriteExt},
    net::TcpStream,
};

use crate::config::Config;
use log::{debug, error};

pub async fn start(config: Config) -> std::io::Result<()> {
    let mut tasks = vec![];
    let (tx, rx) = mpsc::channel::<usize>();

    let start = Instant::now();
    let mut elapsed_vec: Vec<usize> = vec![];

    for server in &config.servers {
        let tx = tx.clone();
        let target_addr = server.clone();
        let config = config.clone();

        let task_handle = tokio::spawn(async move {
            client(config, &target_addr, tx).await.unwrap();
        });
        tasks.push(task_handle);
    }

    // use try_join_all as it will cancel all futures if one errors out
    futures::future::try_join_all(tasks).await?;
    debug!("All tasks completed");

    // since I used tx.clone() it's necessary to drop the original tx
    // or the program will just hang since all receivers are not dropped
    drop(tx);

    for elapsed in rx {
        elapsed_vec.push(elapsed);
    }

    let mut hist = Histogram::new();
    for latency in elapsed_vec.iter() {
        hist.increment(latency.to_owned() as u64)
            .expect("cannot increment");
    }

    let total_messages = config.message_per_connection as usize * config.servers.len();

    let total_elapsed = start.elapsed().as_secs_f64();
    println!("\nRPS: {}\n", total_messages as f64 / total_elapsed);
    // print percentiles from the histogram
    println!(
        "\n\nPercentiles:\np50: {} ms \np90: {} ms \np99: {} ms \np999: {} ms\n\nMinimum: {} ms\nMaximum: {} ms\nMean: {} ms\n",
        hist.percentile(50.0).unwrap(),
        hist.percentile(90.0).unwrap(),
        hist.percentile(99.0).unwrap(),
        hist.percentile(99.9).unwrap(),
        hist.minimum().unwrap(),
        hist.maximum().unwrap(),
        hist.mean().unwrap(),
    );

    Ok(())
}

async fn client(config: Config, target_addr: &SocketAddr, tx: Sender<usize>) -> io::Result<()> {
    let mut send_message_count = 0;
    let mut finish = false;

    let message = "hello world";
    let mut buffer = [0u8; 1024];

    let mut stream = connect(target_addr).await?;

    while !finish {
        send_message_count += 1;
        let start = Instant::now();

        if write(&mut stream, message.as_bytes()).await.is_ok() {
            if let Ok(_) = read(&mut stream, &mut buffer).await {
                let latency = Instant::elapsed(&start).as_millis();
                let resp = std::str::from_utf8(&buffer).ok().unwrap_or("no response");
                debug!("received: {} elapsed {}", resp, latency);
                tx.send(latency as usize)
                    .expect("unable to send latency message");
            }
        } else {
            finish = true;
        }
        if send_message_count == config.message_per_connection {
            finish = true;
        }
    }
    drop(tx);

    Ok(())
}

async fn connect(addr: &SocketAddr) -> io::Result<TcpStream> {
    match TcpStream::connect(addr).await {
        Ok(stream) => {
            debug!("connected to {}", addr);
            Ok(stream)
        }
        Err(e) => {
            if e.kind() != io::ErrorKind::TimedOut {
                error!("unknown connect error: '{}'", e);
            }
            Err(e)
        }
    }
}

async fn write(stream: &mut TcpStream, buf: &[u8]) -> io::Result<usize> {
    match stream.write_all(buf).await {
        Ok(_) => {
            let n = buf.len();
            Ok(n)
        }
        Err(e) => Err(e),
    }
}

async fn read(stream: &mut TcpStream, mut read_buffer: &mut [u8]) -> io::Result<usize> {
    match stream.read(&mut read_buffer).await {
        Ok(n) => Ok(n),
        Err(e) => Err(e),
    }
}

use log::info;
use serde_derive::Deserialize;
use std::net::{IpAddr, Ipv4Addr, SocketAddr};

#[derive(Debug, Clone, Deserialize)]
pub struct Config {
    pub servers: Vec<SocketAddr>,
    pub message_per_connection: u16,
}

#[derive(Debug, Clone, Deserialize)]
struct Ports {
    Ports: Vec<u16>,
}

#[derive(Debug, Clone, Deserialize)]
struct RawConfig {
    Apps: Vec<Ports>,
}

impl Config {
    pub fn new(path: String, messages_per_connection: u16) -> Config {
        let raw_config = {
            let config_file_str = std::fs::read_to_string(&path).expect("unable to read file");
            serde_json::from_str::<RawConfig>(&config_file_str).unwrap()
        };

        let mut servers: Vec<SocketAddr> = vec![];
        for app in raw_config.Apps {
            let ports = app
                .Ports
                .iter()
                .map(|port| {
                    let socket =
                        SocketAddr::new(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)), port.clone());
                    socket
                })
                .collect::<Vec<SocketAddr>>();
            servers.extend(ports);
        }

        info!("using config file: {}", path);
        info!(
            "messages to send per connection: {}",
            messages_per_connection
        );

        return Config {
            servers: servers,
            message_per_connection: messages_per_connection,
        };
    }
}

use proxy_client::{client, config::Config, init_logger};

#[tokio::main]
async fn main() {
    init_logger(log::LevelFilter::Debug).expect("failed to initialize logger");

    let no_of_args = std::env::args().len();
    if no_of_args == 2 {
        println!(
            "must specify two positional arguments\n\
            1 - config file path                      (default: ./config.json)\n\
            2 - no of messages to send per connection (default: 10)\n\n\
        example usage: proxy-client config.json 50\n"
        );
        return;
    }

    let config_path = std::env::args().nth(1).unwrap_or("config.json".to_string());
    let messages_per_connection: u16 = std::env::args()
        .nth(2)
        .map_or(10, |n| n.parse().unwrap_or(10));
    let config = Config::new(config_path, messages_per_connection);

    client::start(config).await.expect("something went wrong");
}

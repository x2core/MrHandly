use thiserror::Error;

/// Errors surfaced by the core crate.
#[derive(Debug, Error)]
pub enum Error {
    #[error("http: {0}")]
    Http(#[from] reqwest::Error),

    #[error("json: {0}")]
    Json(#[from] serde_json::Error),

    #[error("io: {0}")]
    Io(#[from] std::io::Error),

    #[error("toml decode: {0}")]
    TomlDe(#[from] toml::de::Error),

    #[error("toml encode: {0}")]
    TomlSer(#[from] toml::ser::Error),

    #[error("config directory unavailable")]
    NoConfigDir,

    #[error("peer not found: {0}")]
    PeerNotFound(String),
}

pub type Result<T> = std::result::Result<T, Error>;

//! The peer list — the only real state the desktop app owns (CLAUDE.md §6). It
//! is a TOML file; there is no database. A peer's WireGuard address is its
//! stable identity, so the UI keys on `address` and `rename` only changes the
//! display name.

use crate::error::{Error, Result};
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::Path;

/// Default agent port; matches the agent's config default.
pub const DEFAULT_PORT: u16 = 8443;

fn default_port() -> u16 {
    DEFAULT_PORT
}

/// A single fleet member: a display name and a `wg0` address. Nothing else.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Peer {
    pub name: String,
    pub address: String,
    #[serde(default = "default_port")]
    pub port: u16,
}

impl Peer {
    /// Base URL of the peer's agent, e.g. `http://10.44.0.2:8443`.
    pub fn base_url(&self) -> String {
        // IPv6 literals need brackets in a URL authority.
        if self.address.contains(':') && !self.address.starts_with('[') {
            format!("http://[{}]:{}", self.address, self.port)
        } else {
            format!("http://{}:{}", self.address, self.port)
        }
    }
}

/// The on-disk peer list.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct PeersFile {
    #[serde(default)]
    pub peers: Vec<Peer>,
}

impl PeersFile {
    /// Load the peer list, treating a missing file as an empty list.
    pub fn load(path: &Path) -> Result<Self> {
        match fs::read_to_string(path) {
            Ok(s) => Ok(toml::from_str(&s)?),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(Self::default()),
            Err(e) => Err(e.into()),
        }
    }

    /// Persist the peer list, creating parent directories as needed.
    pub fn save(&self, path: &Path) -> Result<()> {
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent)?;
        }
        fs::write(path, toml::to_string_pretty(self)?)?;
        Ok(())
    }

    fn index(&self, address: &str) -> Option<usize> {
        self.peers.iter().position(|p| p.address == address)
    }

    /// Add a peer. Adding is a name and an address — nothing else. Returns an
    /// error if a peer with that address already exists.
    pub fn add(&mut self, name: &str, address: &str, port: u16) -> Result<()> {
        if self.index(address).is_some() {
            return Err(Error::PeerNotFound(format!("{address} already exists")));
        }
        self.peers.push(Peer {
            name: name.to_string(),
            address: address.to_string(),
            port,
        });
        Ok(())
    }

    /// Remove the peer with the given address.
    pub fn remove(&mut self, address: &str) -> Result<()> {
        let i = self
            .index(address)
            .ok_or_else(|| Error::PeerNotFound(address.to_string()))?;
        self.peers.remove(i);
        Ok(())
    }

    /// Change a peer's display name, keyed by its (stable) address.
    pub fn rename(&mut self, address: &str, name: &str) -> Result<()> {
        let i = self
            .index(address)
            .ok_or_else(|| Error::PeerNotFound(address.to_string()))?;
        self.peers[i].name = name.to_string();
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn base_url_ipv4_and_ipv6() {
        let p = Peer {
            name: "a".into(),
            address: "10.44.0.2".into(),
            port: 8443,
        };
        assert_eq!(p.base_url(), "http://10.44.0.2:8443");
        let p6 = Peer {
            name: "b".into(),
            address: "fd00::5".into(),
            port: 9000,
        };
        assert_eq!(p6.base_url(), "http://[fd00::5]:9000");
    }

    #[test]
    fn add_remove_rename() {
        let mut f = PeersFile::default();
        f.add("lab-01", "10.44.0.2", 8443).unwrap();
        f.add("lab-02", "10.44.0.3", 8443).unwrap();
        assert_eq!(f.peers.len(), 2);

        // Duplicate address rejected.
        assert!(f.add("dup", "10.44.0.2", 8443).is_err());

        f.rename("10.44.0.2", "lab-one").unwrap();
        assert_eq!(f.peers[0].name, "lab-one");

        f.remove("10.44.0.3").unwrap();
        assert_eq!(f.peers.len(), 1);
        assert!(f.remove("10.44.0.99").is_err());
    }

    #[test]
    fn load_missing_is_empty() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("peers.toml");
        let f = PeersFile::load(&path).unwrap();
        assert!(f.peers.is_empty());
    }

    #[test]
    fn save_then_load_roundtrips() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("sub/peers.toml"); // parent created on save
        let mut f = PeersFile::default();
        f.add("lab-01", "10.44.0.2", 8443).unwrap();
        f.add("lab-02", "10.44.0.3", 9000).unwrap();
        f.save(&path).unwrap();

        let loaded = PeersFile::load(&path).unwrap();
        assert_eq!(loaded.peers, f.peers);
    }

    #[test]
    fn port_defaults_when_absent() {
        let toml = r#"
[[peers]]
name = "lab-01"
address = "10.44.0.2"
"#;
        let f: PeersFile = toml::from_str(toml).unwrap();
        assert_eq!(f.peers[0].port, DEFAULT_PORT);
    }
}

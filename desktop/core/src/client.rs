//! The agent client. The Rust core owns every socket; the webview never opens
//! one (CLAUDE.md §3). This is a thin async wrapper over the agent's read API:
//! one-shot info/metrics and the metrics SSE stream.

use crate::error::Result;
use crate::types::{Info, Metrics};
use futures_util::{Stream, StreamExt};
use std::time::Duration;

/// An HTTP/SSE client for one agent.
#[derive(Clone)]
pub struct AgentClient {
    http: reqwest::Client,
    base: String,
}

impl AgentClient {
    /// Build a client for the given base URL, e.g. `http://10.44.0.2:8443`.
    pub fn new(base_url: impl Into<String>) -> Self {
        let http = reqwest::Client::builder()
            .connect_timeout(Duration::from_secs(3))
            .build()
            .unwrap_or_default();
        Self {
            http,
            base: base_url.into(),
        }
    }

    /// Fetch the agent's identity and capabilities.
    pub async fn info(&self) -> Result<Info> {
        let info = self
            .http
            .get(format!("{}/v1/info", self.base))
            .send()
            .await?
            .error_for_status()?
            .json::<Info>()
            .await?;
        Ok(info)
    }

    /// Fetch a single metrics frame.
    pub async fn metrics(&self) -> Result<Metrics> {
        let m = self
            .http
            .get(format!("{}/v1/metrics", self.base))
            .send()
            .await?
            .error_for_status()?
            .json::<Metrics>()
            .await?;
        Ok(m)
    }

    /// Cheap reachability probe used for the peer LEDs.
    pub async fn reachable(&self) -> bool {
        self.info().await.is_ok()
    }

    /// Open the metrics SSE stream, yielding one item per frame. The stream ends
    /// when the connection closes; the caller (a Tauri task) drops it to
    /// unsubscribe, which the agent's subscription-driven sampler notices.
    pub fn metrics_stream(&self) -> impl Stream<Item = Result<Metrics>> + 'static {
        let http = self.http.clone();
        let url = format!("{}/v1/metrics/stream", self.base);
        async_stream::try_stream! {
            let resp = http.get(url).send().await?.error_for_status()?;
            let mut bytes = resp.bytes_stream();
            let mut buf: Vec<u8> = Vec::new();
            while let Some(chunk) = bytes.next().await {
                buf.extend_from_slice(&chunk?);
                while let Some(pos) = find_subsequence(&buf, b"\n\n") {
                    let frame: Vec<u8> = buf.drain(..pos + 2).collect();
                    if let Some(m) = parse_sse_metrics(&frame)? {
                        yield m;
                    }
                }
            }
        }
    }
}

/// Extract a `Metrics` from one SSE frame (a `data: {json}` line).
fn parse_sse_metrics(frame: &[u8]) -> Result<Option<Metrics>> {
    let text = String::from_utf8_lossy(frame);
    for line in text.lines() {
        if let Some(json) = line.strip_prefix("data: ") {
            return Ok(Some(serde_json::from_str::<Metrics>(json)?));
        }
    }
    Ok(None)
}

fn find_subsequence(haystack: &[u8], needle: &[u8]) -> Option<usize> {
    haystack.windows(needle.len()).position(|w| w == needle)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::TcpListener;

    /// Serve `body` bytes as a single HTTP response on a fresh port, then close.
    /// Returns the base URL to point an AgentClient at.
    async fn serve_once(response: Vec<u8>) -> String {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        tokio::spawn(async move {
            if let Ok((mut sock, _)) = listener.accept().await {
                let mut scratch = [0u8; 1024];
                let _ = sock.read(&mut scratch).await; // consume the request
                let _ = sock.write_all(&response).await;
                let _ = sock.flush().await;
            }
        });
        format!("http://{addr}")
    }

    fn http_response(content_type: &str, body: &str) -> Vec<u8> {
        format!(
            "HTTP/1.1 200 OK\r\nContent-Type: {ct}\r\nContent-Length: {len}\r\nConnection: close\r\n\r\n{body}",
            ct = content_type,
            len = body.len(),
        )
        .into_bytes()
    }

    const INFO_JSON: &str = r#"{"protocol":1,"agent":{"version":"v1.0.0","commit":"abc"},"host":{"hostname":"lab-02","kernel":"6.1.0","distro":"Debian 12","arch":"amd64","cpus":4,"total_memory":16856244224,"boot_time":1700000000},"capabilities":{"systemd":true,"docker":false}}"#;

    const METRICS_JSON: &str = r#"{"timestamp":1700000000000,"cpu":{"usage":0.25,"per_core":[0.2,0.3]},"memory":{"total":100,"used":40,"free":30,"available":60,"buffers":5,"cached":10,"swap_total":0,"swap_used":0},"load":{"one":0.5,"five":0.4,"fifteen":0.3},"uptime_seconds":123.0,"network":{"eth0":{"rx_bytes":1,"tx_bytes":2,"rx_packets":3,"tx_packets":4}},"disk":{"sda":{"reads":1,"writes":2,"read_sectors":3,"write_sectors":4}}}"#;

    #[tokio::test]
    async fn info_parses() {
        let base = serve_once(http_response("application/json", INFO_JSON)).await;
        let c = AgentClient::new(base);
        let info = c.info().await.unwrap();
        assert_eq!(info.host.hostname, "lab-02");
        assert_eq!(info.host.cpus, 4);
        assert!(info.capabilities.systemd);
        assert!(!info.capabilities.docker);
    }

    #[tokio::test]
    async fn metrics_parses() {
        let base = serve_once(http_response("application/json", METRICS_JSON)).await;
        let c = AgentClient::new(base);
        let m = c.metrics().await.unwrap();
        assert_eq!(m.cpu.usage, 0.25);
        assert_eq!(m.cpu.per_core.len(), 2);
        assert_eq!(m.memory.total, 100);
        assert!(m.network.contains_key("eth0"));
    }

    #[tokio::test]
    async fn reachable_false_when_down() {
        let c = AgentClient::new("http://127.0.0.1:1"); // nothing listens
        assert!(!c.reachable().await);
    }

    #[tokio::test]
    async fn metrics_stream_yields_frames() {
        // Two SSE frames then the connection closes.
        let sse = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\ndata: {METRICS_JSON}\n\ndata: {METRICS_JSON}\n\n"
        );
        let base = serve_once(sse.into_bytes()).await;
        let c = AgentClient::new(base);
        let stream = c.metrics_stream();
        futures_util::pin_mut!(stream);

        let first = stream.next().await.unwrap().unwrap();
        assert_eq!(first.cpu.usage, 0.25);
        let second = stream.next().await.unwrap().unwrap();
        assert_eq!(second.memory.used, 40);
    }

    #[test]
    fn parse_sse_ignores_non_data_lines() {
        assert!(parse_sse_metrics(b": comment\n\n").unwrap().is_none());
    }
}

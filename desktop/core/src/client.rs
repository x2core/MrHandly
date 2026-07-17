//! The agent client. The Rust core owns every socket; the webview never opens
//! one (CLAUDE.md §3). This is a thin async wrapper over the agent's read API:
//! one-shot info/metrics and the metrics SSE stream.

use crate::error::Result;
use crate::types::{Container, ContainerLog, Image, Info, LogLine, Metrics, Process, Service};
use futures_util::{Stream, StreamExt};
use serde::de::DeserializeOwned;
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

    async fn get<T: DeserializeOwned>(&self, path: &str) -> Result<T> {
        let v = self
            .http
            .get(format!("{}{}", self.base, path))
            .send()
            .await?
            .error_for_status()?
            .json::<T>()
            .await?;
        Ok(v)
    }

    async fn post(&self, path: &str) -> Result<()> {
        self.http
            .post(format!("{}{}", self.base, path))
            .send()
            .await?
            .error_for_status()?;
        Ok(())
    }

    /// One-shot reads for the table views (M5).
    pub async fn services(&self) -> Result<Vec<Service>> {
        self.get("/v1/services").await
    }
    pub async fn processes(&self) -> Result<Vec<Process>> {
        self.get("/v1/processes").await
    }
    pub async fn containers(&self) -> Result<Vec<Container>> {
        self.get("/v1/docker/containers").await
    }
    pub async fn images(&self) -> Result<Vec<Image>> {
        self.get("/v1/docker/images").await
    }

    /// Write actions (allowlist enforced by the agent; a 403 surfaces as Err).
    pub async fn service_action(&self, unit: &str, action: &str) -> Result<()> {
        self.post(&format!("/v1/services/{unit}/{action}")).await
    }
    pub async fn container_action(&self, id: &str, action: &str) -> Result<()> {
        self.post(&format!("/v1/docker/containers/{id}/{action}"))
            .await
    }

    /// The metrics SSE stream. Dropping the stream unsubscribes, which the
    /// agent's subscription-driven sampler notices and stops sampling.
    pub fn metrics_stream(&self) -> impl Stream<Item = Result<Metrics>> + 'static {
        self.sse("/v1/metrics/stream")
    }

    /// The process-table SSE stream (2s, subscription-driven on the agent).
    pub fn processes_stream(&self) -> impl Stream<Item = Result<Vec<Process>>> + 'static {
        self.sse("/v1/processes/stream")
    }

    /// The services SSE stream (event-driven on the agent).
    pub fn services_stream(&self) -> impl Stream<Item = Result<Vec<Service>>> + 'static {
        self.sse("/v1/services/stream")
    }

    /// Stream journald logs for a unit.
    pub fn service_logs(
        &self,
        unit: &str,
        follow: bool,
    ) -> impl Stream<Item = Result<LogLine>> + 'static {
        self.sse(&format!(
            "/v1/services/{unit}/logs?follow={}",
            if follow { 1 } else { 0 }
        ))
    }

    /// Stream container logs.
    pub fn container_logs(
        &self,
        id: &str,
        follow: bool,
    ) -> impl Stream<Item = Result<ContainerLog>> + 'static {
        self.sse(&format!(
            "/v1/docker/containers/{id}/logs?follow={}",
            if follow { 1 } else { 0 }
        ))
    }

    /// Generic SSE consumer: GET `path`, yield one deserialized `T` per frame.
    fn sse<T: DeserializeOwned + 'static>(
        &self,
        path: &str,
    ) -> impl Stream<Item = Result<T>> + 'static {
        let http = self.http.clone();
        let url = format!("{}{}", self.base, path);
        async_stream::try_stream! {
            let resp = http.get(url).send().await?.error_for_status()?;
            let mut bytes = resp.bytes_stream();
            let mut buf: Vec<u8> = Vec::new();
            while let Some(chunk) = bytes.next().await {
                buf.extend_from_slice(&chunk?);
                while let Some(pos) = find_subsequence(&buf, b"\n\n") {
                    let frame: Vec<u8> = buf.drain(..pos + 2).collect();
                    if let Some(v) = parse_sse::<T>(&frame)? {
                        yield v;
                    }
                }
            }
        }
    }
}

/// Extract a `T` from one SSE frame (a `data: {json}` line).
fn parse_sse<T: DeserializeOwned>(frame: &[u8]) -> Result<Option<T>> {
    let text = String::from_utf8_lossy(frame);
    for line in text.lines() {
        if let Some(json) = line.strip_prefix("data: ") {
            return Ok(Some(serde_json::from_str::<T>(json)?));
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
        assert!(parse_sse::<Metrics>(b": comment\n\n").unwrap().is_none());
    }

    #[test]
    fn parses_services_json() {
        let json = r#"[{"name":"nginx.service","description":"web","load_state":"loaded","active_state":"active","sub_state":"running","writable":true}]"#;
        let svcs: Vec<Service> = serde_json::from_str(json).unwrap();
        assert_eq!(svcs[0].name, "nginx.service");
        assert!(svcs[0].writable);
    }

    #[test]
    fn parses_process_json() {
        let json = r#"{"pid":101,"ppid":1,"name":"nginx","state":"S","cpu":0.25,"rss":8192000,"threads":4}"#;
        let p: Process = serde_json::from_str(json).unwrap();
        assert_eq!(p.pid, 101);
        assert_eq!(p.threads, 4);
    }
}

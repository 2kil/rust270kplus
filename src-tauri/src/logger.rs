use anyhow::Result;
use tracing_subscriber::{fmt, EnvFilter};
pub fn init() -> Result<()> { let _=fmt().with_env_filter(EnvFilter::from_default_env().add_directive("info".parse()?)).try_init(); Ok(()) }

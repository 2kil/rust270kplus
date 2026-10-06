use thiserror::Error;

#[derive(Debug, Error)]
pub enum AppError {
    #[error("配置错误: {0}")]
    Config(String),
    #[error("IO 错误: {0}")]
    Io(#[from] std::io::Error),
}

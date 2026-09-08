fn main() {
    if std::env::var("PROFILE").as_deref() == Ok("release") {
        let manifest = include_str!("manifest.xml");
        let windows = tauri_build::WindowsAttributes::new().app_manifest(manifest);
        tauri_build::try_build(tauri_build::Attributes::new().windows_attributes(windows))
            .expect("Tauri 构建失败");
    } else {
        tauri_build::try_build(tauri_build::Attributes::new()).expect("Tauri 构建失败");
    }
}

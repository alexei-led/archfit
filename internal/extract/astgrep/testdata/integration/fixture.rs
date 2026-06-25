// Minimal Rust fixture for per-rule-file integration tests.
// Covers: rs-func, rs-struct, rs-enum, rs-trait, rs-impl, rs-mod, rs-attribute.
// The #[derive(...)] attribute exercises the previously-broken rs-attribute rule.

#[derive(Debug, Clone)]
pub struct Widget {
    pub name: String,
}

pub enum Color {
    Red,
    Green,
    Blue,
}

pub trait Render {
    fn render(&self) -> String;
}

impl Widget {
    pub fn new(name: String) -> Self {
        Widget { name }
    }
}

pub fn create_widget(name: String) -> Widget {
    Widget::new(name)
}

pub mod shapes {
    pub struct Circle {
        pub radius: f64,
    }
}

// Minimal Rust fixture for per-rule-file integration tests.
// Covers: rs-func, rs-struct, rs-enum, rs-trait, rs-impl, rs-mod, rs-attribute,
//         rs-test-import-mockall (test_import fact).
// The #[derive(...)] attribute exercises the previously-broken rs-attribute rule.

use mockall::automock;

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

// Safety surface fixture: exercises rs-unsafe-block, rs-unsafe-cell, rs-raw-cast, rs-transmute.
use std::cell::UnsafeCell;

pub struct SharedState {
    inner: UnsafeCell<u32>,
}

pub unsafe fn raw_bytes(ptr: *const u8) -> u8 {
    unsafe { *ptr }
}

pub fn raw_cast_example(val: u32) -> *const u32 {
    val as *const u32
}

pub fn transmute_example(val: u64) -> u64 {
    // SAFETY: u64 → u64 is trivially safe; exercises rs-transmute detection.
    unsafe { std::mem::transmute(val) }
}

// MultiField exercises rs-struct-field (struct with named field_declaration_list).
pub struct MultiField {
    pub alpha: u32,
    pub beta: String,
    pub gamma: bool,
}

// Global-state fixture: exercises rs-static-mut, rs-static-atomic, rs-static-oncelock.
// AtomicU32 ID-generator is idiomatic Rust — captured as info signal, not a violation.
use std::sync::atomic::AtomicU32;
use std::sync::OnceLock;

static mut GLOBAL_COUNTER: u32 = 0;
static ID_GENERATOR: AtomicU32 = AtomicU32::new(0);
static REGISTRY: OnceLock<Vec<String>> = OnceLock::new();

// Panic-op fixture: exercises rs-unwrap, rs-expect, rs-panic.
pub fn fixture_panics() -> String {
    let v: Option<String> = None;
    let _ = v.unwrap();           // rs-unwrap
    let _ = v.expect("oops");     // rs-expect
    panic!("fixture panic");      // rs-panic
}

// Test-function fixture: exercises rs-test-fn (test_fn kind).
// Name starts with test_ to match the rs-test-fn rule's regex.
fn test_create_widget() {
    let w = create_widget("hello".to_string());
    let _ = w;
}

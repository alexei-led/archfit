package analysis

// AugmentConfig exposes the synthetic-module augmentation Analyze applies, so
// the registration rules can be pinned without running a full classification.
// It is unexported in production: Analyze and Classify are its only callers.
var AugmentConfig = augmentConfig

// BCAdvisoryWhy exposes the coupling advisory text, so its wording can be
// pinned per severity and distance without a full classification.
var BCAdvisoryWhy = bcAdvisoryWhy

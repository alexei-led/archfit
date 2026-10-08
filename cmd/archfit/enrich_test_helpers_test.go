package main

import (
	"github.com/alexei-led/archfit/v3/internal/labels/labelsio"
	"github.com/alexei-led/archfit/v3/internal/relationship/labels"
)

func writeLabels(path string, in []labels.Label) error { return labelsio.Write(path, in) }

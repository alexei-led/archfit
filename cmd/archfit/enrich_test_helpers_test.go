package main

import (
	"github.com/alexei-led/archfit/internal/labels/labelsio"
	"github.com/alexei-led/archfit/internal/relationship/labels"
)

func writeLabels(path string, in []labels.Label) error { return labelsio.Write(path, in) }

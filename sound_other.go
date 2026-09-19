//go:build !windows

package main

import "errors"

func playSound(string) error { return errors.New("ses önizleme sadece Windows'ta") }

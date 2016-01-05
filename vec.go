package main

import (
	"math"
)

type vec3 [3]int

func vec3Equal(a, b vec3) bool {
	if len(a) != len(b) {
		return false
	}
	for i, c := range a {
		if c != b[i] {
			return false
		}
	}
	return true
}

func vec3Add(a, b vec3) vec3 {
	var out vec3
	for i, c := range a {
		out[i] = c + b[i]
	}
	return out
}

func vec3Sub(a, b vec3) vec3 {
	var out vec3
	for i, c := range a {
		out[i] = c - b[i]
	}
	return out
}

// Calculates L1 (manhattan) distance between two vectors.
func vec3L1Dist(a, b vec3) int {
	dist := float64(0)
	for i, c := range a {
		dist += math.Abs(float64(c - b[i]))
	}
	return int(dist)
}

// Returns one of the dimensions this vector has a non-zero component in.
func vec3Dim(v vec3) int {
	for i, c := range v {
		if c != 0 {
			return i
		}
	}
	return 0
}

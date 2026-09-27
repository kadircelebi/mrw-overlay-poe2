//go:build !store

package main

// storeBuild is false in the GitHub release: the app updates itself from
// GitHub Releases. See storebuild_store.go.
const storeBuild = false

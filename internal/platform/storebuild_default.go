//go:build !store

package platform

// StoreBuild is false in the GitHub release: the app updates itself from
// GitHub Releases. See storebuild_store.go.
const StoreBuild = false

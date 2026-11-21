// SPDX-License-Identifier: GPL-3.0-or-later
// Package protonnode is the gomobile-facing Android core of the Proton node.
//
// P1 scope (PROMPT.md 5.1 subset): Init, Shutdown, Login, Logout, SetSession,
// SessionJSON, SessionState, ListServers, Start, Stop, Status, RecentLogs.
// Scanning (P3), config import/export (P5) and the foreground service (P4) are
// added when their phase starts; absent functions are better than stubs.
//
// Rules that hold for every exported function:
//   - only string/bool/error/int (gomobile cannot bind channels or complex
//     types; structured data is JSON);
//   - blocking calls take a context deadline and return in bounded time;
//   - Status is safe from any thread and never blocks on the network;
//   - the package writes no credentials to disk (the app encrypts SessionJSON
//     itself with Android Keystore, see store.Store).
package protonnode

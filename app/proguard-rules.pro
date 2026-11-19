# Keep the gomobile bridge and the classes reached from JNI by name.
# The list is filled in when the core AAR lands (P1); it must never shrink the
# exported Go symbols.
-keep class io.github.dsh.protonnode.core.** { *; }
# gomobile's generated Java front-end (package name = the Go package).
-keep class protonnode.** { *; }
-keep class go.** { *; }

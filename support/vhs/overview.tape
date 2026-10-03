Source "support/vhs/_config.tape"

Hide
Type@1ms "rm -f $TODO_FILE && clear"
Enter
Type@1ms "todo add document installation +docs"
Enter
Type@1ms "todo add fix slow startup +bug +now"
Enter
Type@1ms "todo add review dependencies +now"
Enter
Type@1ms "todo add improve error messages +ux"
Enter
Type@1ms "todo add test fresh installation +release"
Enter
Type@1ms "clear"
Enter
Show

Type "todo add prepare release +release"
Enter
Sleep 1s
Type "todo ls"
Enter
Sleep 3s

Hide
Type@1ms "clear"
Enter
Show
Type "todo ls +now"
Enter
Sleep 2s
Type "todo ls +now -bug"
Enter
Sleep 2s
Type "todo ls +docs/+release"
Enter
Sleep 3s

Hide
Type@1ms "clear"
Enter
Show
Type "todo edit 1 document installation and usage"
Enter
Sleep 2s
Type "todo edit 5 +6"
Enter
Sleep 2s
Type "todo ls 6/+6"
Enter
Sleep 3s

Hide
Type@1ms "clear"
Enter
Show
Type "todo edit 2/3 -now +done"
Enter
Sleep 2s
Type "todo ls +done"
Enter
Sleep 2s
Hide
Type@1ms "clear"
Enter
Show
Type "todo del +done"
Enter
Sleep 2s
Type "todo ls"
Enter
Sleep 5s

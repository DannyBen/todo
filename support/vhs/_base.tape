Set Shell "bash"
Set FontSize 28
Set Width 1000
Set Height 600
Set Padding 20
Set TypingSpeed 70ms

Env TODO_FILE "support/vhs/.todo.sqlite"
Env PS1 "\n$ "
Env NO_COLOR ""

Hide
Type@1ms "rm -f $TODO_FILE && clear"
Enter
Type@1ms "todo add document installation options +docs"
Enter
Type@1ms "todo add investigate slow startup +bug +now"
Enter
Type@1ms "clear"
Enter
Show
Sleep 1s

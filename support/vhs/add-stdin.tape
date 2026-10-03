Source "support/vhs/_config.tape"
Source "support/vhs/_seed.tape"

Type `echo "Write tests +now" > todo.txt`
Enter
Sleep 1s
Type `echo "Update docs +maybe" >> todo.txt`
Enter
Sleep 1s
Type "cat todo.txt"
Enter
Sleep 2s
Type "todo add < todo.txt"
Enter
Sleep 5s

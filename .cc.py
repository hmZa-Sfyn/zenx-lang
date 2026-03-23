import os

for x in os.listdir():
	if x == "examples":
		for y in os.listdir(x):
			# source /home/hmza/myenv/bin/activate
			print(y)
			os.system(f"./zxc ./examples/{y} -o /tmp/123")

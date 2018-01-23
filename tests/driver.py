FILENAME="tests.txt"
import requests
import time
import sys

f = open(FILENAME)

for line in f:
    if len(line) > 0:
        r = requests.get(line)
        time.sleep(5)
        print (r.text)
        i = input("Do you want to continue? [y/n]")
        if i == "n":
            sys.exit(1)


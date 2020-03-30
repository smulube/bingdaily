# bingdaily

Simple little CLI tool that allows you to programmatically change the
background on a Linux system that happens to be using Gnome. I've only tested
on Ubuntu.

The intention for this tool is to run it from a cron job meaning your desktop
will automatically get a new background every day. For the tool to be able to
update your wallpaper it requires the presence of a couple of environment
variables. An example crontab is shown below:

```
DISPLAY=:0
GSETTINGS_BACKEND=dconf
# m h  dom mon dow   command
0 * * * * /workspace/go/bin/bingdaily > /dev/null 2>&1
```


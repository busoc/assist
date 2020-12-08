package main

const helpText = `ASIM Semi Automatic Schedule Tool

Usage: assist [options] <trajectory.csv>

Command files:

assist accepts command files by pair. In other words, if the ROCON file is given,
the ROCOFF should also be provided. The same is true for the CERON/CEROFF files.

However, it is not mandatory to have the 4 files provided. A schedule can be
created only for ROC or for CER (see examples below).

It is an error to not provide any file unless if the list flag is given to assist.

Input format:

the input of assist consists of a tabulated "file". The columns of the file are:

- datetime (YYYY-mm-dd HH:MM:SS.ssssss)
- modified julian day
- altitude (kilometer)
- latitude (degree or DMS)
- longitude (degree or DMS)
- eclipse (1: night, 0: day)
- crossing (1: crossing, 0: no crossing)
- TLE epoch

assist only uses the columns from the input file (but all are mandatory even if
empty):

- datetime
- eclipse
- crossing

the values accepted by assist to decide if the trajectory is "entering" SAA/
Eclipse, are: 1, on, true

the values accepted by assist to decide if the trajectory is "leaving" SAA/
Eclipse are: 0, off, false

Configuration sections/options:

assist accepts via the "config" flag a configuration file as first argument
instead of a file with a predicted trajectory. The format of the configuration
file is toml.

There are three main sections in the configuration files (options for each section
are described below - check also the Options section of this help for additional
information):

* default : configuring the input and output of assist
  - alliop       = file where schedule file will be created
  - instrlist    = file where instrlist file will be created
  - path         = file with the input trajectory to use to create the schedule
	- resolution   = time interval between two rows in the trajectory file
  - keep-comment = schedule contains the comment present in the command files

* delta   : configuring the various time used to schedule the ROC and CER commands
  - wait           = wait time after entering eclipse for ROCON to be scheduled
  - azm            = duration of the AZM
  - rocon          = expected time of the ROCON
  - rocoff         = expected time of the ROCOFF
  - margin         = minium interval of time between ROCON end and ROCOFF start
  - cer            = time before entering eclipse to activate CER(ON|OFF)
  - cer-before     = time before SAA during eclipse to schedule CERON
  - cer-after      = time after SAA during eclipse to schedule CEROFF
  - cer-before-roc = time before ROCON/ROCOFF to schedule a CERON
  - cer-after-roc  = time after ROCON/ROCOFF to schedule a CEROFF
  - crossing       = mininum time of SAA and Eclipse
  - saa            = mininum SAA duration to have an AZM scheduled

* commands: configuring the location of the files that contain the commands
  - rocon  = file with commands for ROCON in text format
  - rocoff = file with commands for ROCOFF in text format
  - ceron  = file with commands for CERON in text format
  - ceroff = file with commands for CEROFF in text format

Options:

  -rocon-time     TIME  ROCON expected execution time
  -rocoff-time    TIME  ROCOFF expected execution time
  -rocon-wait     TIME  wait TIME after entering Eclipse before starting ROCON
  -roc-margin     TIME  margin time between ROCON end and ROCOFF start
  -cer-time       TIME  TIME before Eclipse to switch CER(ON|OFF)
  -cer-crossing   TIME  minimum crossing time of SAA and Eclipse to switch CER(ON|OFF)
  -cer-before     TIME  schedule CERON TIME before entering SAA during eclipse
  -cer-after      TIME  schedule CEROFF TIME before leaving SAA during eclipse
  -cer-before-roc TIME  delay CERON before ROC when conflict
  -cer-after-roc  TIME  delay CEROFF after ROC when conflict
  -azm            TIME  AZM duration
  -saa            TIME  SAA duration
  -acs-time       TIME  ACS expected execution time
  -acs-night      TIME  ACS minimum night duration
  -rocon-file     FILE  use FILE with commands for ROCON
  -rocoff-file    FILE  use FILE with commands for ROCOFF
  -ceron-file     FILE  use FILE with commands for CERON
  -ceroff-file    FILE  use FILE with commands for CEROFF
  -acson-file     FILE  use FILE with commands for ACSON
  -acsoff-file    FILE  use FILE with commands for ACSOFF
  -resolution     TIME  TIME interval between two rows in the trajectory
  -base-time      DATE
  -alliop         FILE  save schedule to FILE
  -instrlist      FILE  save instrlist to FILE
  -keep-comment         keep comment (if any) from command files
  -list-periods         print the list of eclipses and crossing periods
  -list                 print the list of commands instead of creating a schedule
  -ignore               keep schedule entries from block that does not meet constraints
  -config               load settings from a configuration file
  -version              print assist version and exit
  -help                 print this message and exit

Examples:

# create a full schedule keeping the comments from the given command files with
# a longer AZM for ROCON/ROCOFF and a larger margin for CERON/CEROFF
# this command enable the classical algorithm to schedule CERON/CEROFF
$ assist -keep-comment -base-time 2018-11-19T12:35:00Z \
  -rocon-file /usr/local/etc/asim/MXGS-ROCON.txt \
  -rocoff-file /usr/local/etc/asim/MXGS-ROCOFF.txt \
  -ceron-file /usr/local/etc/asim/MMIA-CERON.txt \
  -ceroff-file /usr/local/etc/asim/MMIA-CEROFF.txt \
  -azm 80s \
  -cer-time 900s \
  -alliop /var/asim/2018-310/alliop.txt \
  -instrlist /var/asim/2018-310/instrlist.txt \
  inspect-trajectory.csv

# create a schedule only for CER (the same can be done for ROC).
# this command enable the classical algorithm to schedule CERON/CEROFF
$ assist -keep-comment -base-time 2018-11-19T12:35:00Z \
  -ceron-file /usr/local/etc/asim/MMIA-CERON.txt \
  -ceroff-file /usr/local/etc/asim/MMIA-CEROFF.txt \
  -cer-time 900s \
  -alliop /var/asim/CER-2018-310/alliop.txt \
  inspect-trajectory.csv

# Same command as previous but this time CERON/CEROFF will be scheduled according
# to the new algorithm (using the default value for the cer-before, cer-after,...
# options).
$ assist -keep-comment -base-time 2018-11-19T12:35:00Z \
  -ceron-file /usr/local/etc/asim/MMIA-CERON.txt \
  -ceroff-file /usr/local/etc/asim/MMIA-CEROFF.txt \
  -alliop /var/asim/CER-2018-310/alliop.txt \
  inspect-trajectory.csv

# print the list of commands that could be scheduled from a local file
$ assist -list tmp/inspect-trajectory.csv

# print the list of commands that could be scheduled from the output of inspect
$ inspect -d 24h -i 10s -f csv /tmp/tle-2018305.txt | assist -list

# use a configuration file instead of command line options
$ assist -config /usr/local/etc/asim/cerroc-ops.toml
`

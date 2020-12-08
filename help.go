package main

const helpText = `ASIM Semi Automatic Schedule Tool

Usage: assist [options] <config.toml>

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
  - acs-time       = ACS expected execution time
  - acs-night      = ACS minimum night duration

* commands: configuring the location of the files that contain the commands
  - rocon  = file with commands for ROCON in text format
  - rocoff = file with commands for ROCOFF in text format
  - ceron  = file with commands for CERON in text format
  - ceroff = file with commands for CEROFF in text format
  - acson  = file with commands for ACSON in text format
  - acsoff = file with commands for ACSOFF in text format

Options:

  -list-periods  print the list of eclipses and crossing periods
  -list-entries  print the list of commands instead of creating a schedule
  -version       print assist version and exit
  -help          print this message and exit
`

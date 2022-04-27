The ASIM Semi Automatic Schedule Generator Tool has been developped to help operators
to create command schedules in order to put instruments in a correct configuration
for ISS day and SAA passes.

Assist goal is to build the command text files necessary for the on-board daily
schedule execution. This daily schedule ensures that science is maximized during
the eclipse passes and that the instruments are in a correct configuration for
eclipse and SAA passes.

# scheduling commands

As state above, assist is used to schedule the commands execution on board. To
perform this task, it uses different "algorithm" to schedule MXGS and MMIA. This
section describes briefly how assist works for both of the instruments.

## scheduling ROCON/ROCOFF (MXGS)

MXGS should be put on at the beginning of each ISS night and off at the end of each
of ISS night.

However, assist should take into account crossing of SAA during the night. Indeed,
when a SAA crossing is detected, another block of commands (AZM) should be executed
before the ROCOON/ROCOFF. The corner cases are when the crossing of SAA occurs at
the beginning or at the end of the ISS night.

Another additional case taken into account is that assist can not schedule a
ROCON/ROCOFF pair if the duration of the night is too short (this value is
specified in the options)

## scheduling CERON/CEROFF (MMIA)

CERON are scheduled by assist before the first night where there is a SAA pass
longer than a configured value.

CEROFF are scheduled by assist before the first night where there is no SAA pass
longer than a configured value.

# assist input

assist takes as input a csv file (if the config option is not set) that contains
a predicted trajectory for the ISS. Such kind of files can be generated via the
[inspect](https://github.com/busoc/inspect) tool.

This file should have the following columns:

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

# usage

```
$ assist [options] <trajectory|configuration>

where options are:

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
  -rocon-file     FILE  use FILE with commands for ROCON
  -rocoff-file    FILE  use FILE with commands for ROCOFF
  -ceron-file     FILE  use FILE with commands for CERON
  -ceroff-file    FILE  use FILE with commands for CEROFF
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
```

# configuring Assist

assist accepts via the "config" flag a configuration file as first argument
instead of a file with a predicted trajectory. The format of the configuration
file is toml.

There are three main tables in the configuration files (options for each section
are described below - check also the Options section of this help for additional
information):

## table []

* alliop       = file where schedule file will be created
* instrlist    = file where instrlist file will be created
* path         = file with the input trajectory to use to create the schedule
* resolution   = time interval between two rows in the trajectory file
* keep-comment = schedule contains the comment present in the command files

## table [delta]

the delta table is used to specify the various delay, delta, duration that assist
should take into account in order to set the correct time for the execution of
the commands

* wait           = wait time after entering eclipse for ROCON to be scheduled
* azm            = duration of the AZM
* rocon          = expected time of the ROCON
* rocoff         = expected time of the ROCOFF
* margin         = minium interval of time between ROCON end and ROCOFF start
* cer            = time before entering eclipse to activate CER(ON|OFF)
* cer-before     = time before SAA during eclipse to schedule CERON
* cer-after      = time after SAA during eclipse to schedule CEROFF
* cer-before-roc = time before ROCON/ROCOFF to schedule a CERON
* cer-after-roc  = time after ROCON/ROCOFF to schedule a CEROFF
* crossing       = mininum time of SAA and Eclipse
* saa            = mininum SAA duration to have an AZM scheduled

## table [commands]

the commands table specified the path to the various command file that are used
as "template" by assist

* rocon  = file with commands for ROCON in text format
* rocoff = file with commands for ROCOFF in text format
* ceron  = file with commands for CERON in text format
* ceroff = file with commands for CEROFF in text format

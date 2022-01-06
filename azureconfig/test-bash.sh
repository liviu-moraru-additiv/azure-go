#!/bin/bash

usage() {
  echo "Usage: $0  -u NAME  -a AGE " 1>&2 
}

exit_abnormal() {
  usage
  exit 1
}

while getopts u:a:f: flag
do
    case "${flag}" in
        u) username=${OPTARG};;
        a) age=${OPTARG};;
        f) fullname=${OPTARG};;
    esac
done
groupName=appconfig-rg
if [ "${username}" == '' ]
    then 
        exit_abnormal 
fi
echo "Username: $username";
echo "Age: $age";
echo "Full Name: $fullname";
echo ${groupName}


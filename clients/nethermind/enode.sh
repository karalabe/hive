#!/bin/bash

# Script to retrieve the enode
# 
# This is copied into the validator container by Hive
# and used to provide a client-specific enode id retriever
#

# Immediately abort the script on any error encountered


set -e

TARGET_ENODE=$(sed -n -e 's/^.*\(enode.*\) $/\1/p' /log.txt | head -n 1)
echo $TARGET_ENODE


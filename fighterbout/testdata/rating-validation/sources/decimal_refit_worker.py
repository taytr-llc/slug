#!/usr/bin/env python3
"""Owned single-fit Decimal worker. Main runner enforces deadline and joins it."""
import sys
import execution as E
from coefficient_study import decimal_fit
if __name__=='__main__':
 E.gate();r=int(sys.argv[1]);assert -1<=r<1000;decimal_fit(r)

"""Pytest configuration ensuring ai-tier is on sys.path."""

import os
import sys

sys.path.insert(0, os.path.abspath(os.path.dirname(__file__)))

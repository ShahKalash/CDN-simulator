#!/usr/bin/env python3
"""
Main simulation runner
Executes all simulations, generates visualizations, and creates report
"""

import sys
import os
import time

def main():
    print("=" * 80)
    print("IMMUNE-BASED EDGE PLACEMENT SIMULATION")
    print("Hybrid CDN/P2P Content Delivery Network")
    print("=" * 80)
    print()
    
    # Step 1: Run simulations
    print("STEP 1: Running simulations...")
    print("-" * 80)
    from simulation_runner import SimulationRunner
    runner = SimulationRunner()
    results = runner.run_all_simulations()
    print()
    
    # Step 2: Generate visualizations
    print("STEP 2: Generating visualizations...")
    print("-" * 80)
    try:
        from visualize_metrics import MetricsVisualizer
        visualizer = MetricsVisualizer()
        visualizer.generate_all_plots()
    except ImportError as e:
        print(f"Warning: Could not generate visualizations: {e}")
        print("Make sure matplotlib is installed: pip install matplotlib")
    print()
    
    # Step 3: Generate report
    print("STEP 3: Generating report...")
    print("-" * 80)
    try:
        from generate_report import ReportGenerator
        generator = ReportGenerator()
        generator.generate_report()
    except Exception as e:
        print(f"Warning: Could not generate report: {e}")
    print()
    
    print("=" * 80)
    print("SIMULATION COMPLETE!")
    print("=" * 80)
    print()
    print("Results:")
    print("  - Simulation data: immune_placement_simulation/simulation_results.json")
    print("  - Visualizations: immune_placement_simulation/figures/")
    print("  - Report: immune_placement_simulation/IMMUNE_PLACEMENT_REPORT.md")
    print()
    print("=" * 80)

if __name__ == "__main__":
    main()


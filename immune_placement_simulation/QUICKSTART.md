# Quick Start Guide

## Installation

```bash
# Install Python dependencies
pip install -r requirements.txt
```

## Run Complete Simulation

```bash
# Run all simulations, generate visualizations, and create report
python run_simulation.py
```

This single command will:
1. ✅ Run baseline comparison
2. ✅ Run parameter sensitivity analysis  
3. ✅ Run scalability tests
4. ✅ Generate all visualizations
5. ✅ Create comprehensive report

## Expected Runtime

- **Total Time**: ~2-5 minutes
- **Baseline Comparison**: ~30 seconds
- **Parameter Sensitivity**: ~60 seconds
- **Scalability Test**: ~90 seconds
- **Visualization**: ~10 seconds
- **Report Generation**: ~5 seconds

## Output Files

After running, you'll find:

```
immune_placement_simulation/
├── simulation_results.json          # Raw simulation data
├── IMMUNE_PLACEMENT_REPORT.md       # Comprehensive report
└── figures/
    ├── baseline_comparison.png
    ├── convergence.png
    ├── parameter_sensitivity.png
    ├── scalability.png
    └── edge_load_distribution.png
```

## View Results

1. **Read the Report**: Open `IMMUNE_PLACEMENT_REPORT.md`
2. **View Charts**: Check `figures/` directory
3. **Analyze Data**: Open `simulation_results.json`

## Customization

### Change Network Size

Edit `simulation_runner.py`:
```python
peers, rtt_matrix = generate_synthetic_network(num_peers=200, seed=42)
```

### Change Algorithm Parameters

Edit `immune_placement.py`:
```python
algorithm = ImmunePlacementAlgorithm(
    num_super_peers=20,      # More super peers
    alpha=0.8,               # More weight on delay
    beta=0.2,                # Less weight on load balance
    pop_size=50,             # Larger population
    max_generations=150      # More iterations
)
```

## Troubleshooting

### Import Errors
```bash
pip install numpy matplotlib
```

### Missing Figures Directory
The script will create it automatically, but you can create manually:
```bash
mkdir immune_placement_simulation/figures
```

### Memory Issues
Reduce network size or population size in the code.

## Quick Test

To run a quick test (faster, less comprehensive):
```python
from immune_placement import *
peers, rtt_matrix = generate_synthetic_network(num_peers=50, seed=42)
algorithm = ImmunePlacementAlgorithm(peers=peers, rtt_matrix=rtt_matrix, max_generations=20)
best_solution, history = algorithm.optimize()
print(f"Best delay: {best_solution.avg_delay:.2f}ms")
```

## Next Steps

1. Review the generated report
2. Analyze the visualizations
3. Integrate results into your hybrid CDN/P2P system
4. Update your project documentation with findings

---

**Need Help?** Check `README.md` for detailed documentation.


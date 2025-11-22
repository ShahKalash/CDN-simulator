"""
Metrics Visualization for Immune-Based Edge Placement Simulation
Generates charts and graphs for analysis
"""

import json
import matplotlib.pyplot as plt
import numpy as np
from typing import Dict, List
import os


class MetricsVisualizer:
    """Creates visualizations from simulation results"""
    
    def __init__(self, results_file: str = 'simulation_results.json'):
        with open(results_file, 'r') as f:
            self.results = json.load(f)
        self.output_dir = 'figures'
        os.makedirs(self.output_dir, exist_ok=True)
    
    def plot_baseline_comparison(self):
        """Plot comparison between baseline methods and optimized solution"""
        baseline = self.results['simulations']['baseline_comparison']
        
        methods = ['Round-Robin', 'Nearest Edge', 'Random Super Peers', 'Immune Optimized']
        avg_delays = [
            baseline['baselines']['round_robin']['avg_delay_ms'],
            baseline['baselines']['nearest_edge']['avg_delay_ms'],
            baseline['baselines']['random_super_peers']['avg_delay_ms'],
            baseline['optimized']['avg_delay_ms']
        ]
        load_imbalances = [
            baseline['baselines']['round_robin']['load_imbalance'],
            baseline['baselines']['nearest_edge']['load_imbalance'],
            baseline['baselines']['random_super_peers']['load_imbalance'],
            baseline['optimized']['load_imbalance']
        ]
        p2p_hit_rates = [
            baseline['baselines']['round_robin']['p2p_hit_rate'],
            baseline['baselines']['nearest_edge']['p2p_hit_rate'],
            baseline['baselines']['random_super_peers']['p2p_hit_rate'],
            baseline['optimized']['p2p_hit_rate']
        ]
        
        fig, axes = plt.subplots(1, 3, figsize=(18, 5))
        
        # Average Delay
        axes[0].bar(methods, avg_delays, color=['#ff6b6b', '#4ecdc4', '#45b7d1', '#96ceb4'])
        axes[0].set_ylabel('Average Delay (ms)', fontsize=12)
        axes[0].set_title('Average Access Delay Comparison', fontsize=14, fontweight='bold')
        axes[0].grid(axis='y', alpha=0.3)
        axes[0].tick_params(axis='x', rotation=15)
        
        # Load Imbalance
        axes[1].bar(methods, load_imbalances, color=['#ff6b6b', '#4ecdc4', '#45b7d1', '#96ceb4'])
        axes[1].set_ylabel('Load Imbalance (variance)', fontsize=12)
        axes[1].set_title('Load Balancing Comparison', fontsize=14, fontweight='bold')
        axes[1].grid(axis='y', alpha=0.3)
        axes[1].tick_params(axis='x', rotation=15)
        
        # P2P Hit Rate
        axes[2].bar(methods, [r * 100 for r in p2p_hit_rates], color=['#ff6b6b', '#4ecdc4', '#45b7d1', '#96ceb4'])
        axes[2].set_ylabel('P2P Hit Rate (%)', fontsize=12)
        axes[2].set_title('P2P Cache Hit Rate', fontsize=14, fontweight='bold')
        axes[2].grid(axis='y', alpha=0.3)
        axes[2].tick_params(axis='x', rotation=15)
        
        plt.tight_layout()
        plt.savefig(f'{self.output_dir}/baseline_comparison.png', dpi=300, bbox_inches='tight')
        plt.close()
        print("Saved: baseline_comparison.png")
    
    def plot_convergence(self):
        """Plot convergence history of immune algorithm"""
        baseline = self.results['simulations']['baseline_comparison']
        history = baseline['optimized']['convergence_history']
        
        generations = [h['generation'] for h in history]
        best_fitness = [h['best_fitness'] for h in history]
        avg_fitness = [h['avg_fitness'] for h in history]
        avg_delay = [h['best_avg_delay'] for h in history]
        load_imbalance = [h['best_load_imbalance'] for h in history]
        
        fig, axes = plt.subplots(2, 2, figsize=(16, 10))
        
        # Fitness convergence
        axes[0, 0].plot(generations, best_fitness, 'b-', linewidth=2, label='Best Fitness')
        axes[0, 0].plot(generations, avg_fitness, 'r--', linewidth=1.5, label='Average Fitness')
        axes[0, 0].set_xlabel('Generation', fontsize=12)
        axes[0, 0].set_ylabel('Fitness', fontsize=12)
        axes[0, 0].set_title('Fitness Convergence', fontsize=14, fontweight='bold')
        axes[0, 0].legend()
        axes[0, 0].grid(alpha=0.3)
        
        # Average delay convergence
        axes[0, 1].plot(generations, avg_delay, 'g-', linewidth=2)
        axes[0, 1].set_xlabel('Generation', fontsize=12)
        axes[0, 1].set_ylabel('Average Delay (ms)', fontsize=12)
        axes[0, 1].set_title('Average Delay Optimization', fontsize=14, fontweight='bold')
        axes[0, 1].grid(alpha=0.3)
        
        # Load imbalance convergence
        axes[1, 0].plot(generations, load_imbalance, 'm-', linewidth=2)
        axes[1, 0].set_xlabel('Generation', fontsize=12)
        axes[1, 0].set_ylabel('Load Imbalance', fontsize=12)
        axes[1, 0].set_title('Load Balancing Optimization', fontsize=14, fontweight='bold')
        axes[1, 0].grid(alpha=0.3)
        
        # Edge loads
        edge_a_loads = [h['best_edge_a_load'] for h in history]
        edge_b_loads = [h['best_edge_b_load'] for h in history]
        axes[1, 1].plot(generations, edge_a_loads, 'b-', linewidth=2, label='Edge A Load')
        axes[1, 1].plot(generations, edge_b_loads, 'r-', linewidth=2, label='Edge B Load')
        axes[1, 1].set_xlabel('Generation', fontsize=12)
        axes[1, 1].set_ylabel('Load (kbps)', fontsize=12)
        axes[1, 1].set_title('Edge Server Load Balancing', fontsize=14, fontweight='bold')
        axes[1, 1].legend()
        axes[1, 1].grid(alpha=0.3)
        
        plt.tight_layout()
        plt.savefig(f'{self.output_dir}/convergence.png', dpi=300, bbox_inches='tight')
        plt.close()
        print("Saved: convergence.png")
    
    def plot_parameter_sensitivity(self):
        """Plot parameter sensitivity analysis"""
        sensitivity = self.results['simulations']['parameter_sensitivity']
        
        # Alpha/Beta sensitivity
        alpha_beta = sensitivity['alpha_beta_tests']
        alphas = [t['alpha'] for t in alpha_beta]
        delays = [t['metrics']['avg_delay_ms'] for t in alpha_beta]
        imbalances = [t['metrics']['load_imbalance'] for t in alpha_beta]
        
        # Super peer sensitivity
        super_peer = sensitivity['super_peer_tests']
        num_sp = [t['num_super_peers'] for t in super_peer]
        sp_delays = [t['metrics']['avg_delay_ms'] for t in super_peer]
        sp_p2p_hits = [t['metrics']['p2p_hit_rate'] * 100 for t in super_peer]
        
        fig, axes = plt.subplots(2, 2, figsize=(16, 10))
        
        # Alpha vs Delay
        axes[0, 0].plot(alphas, delays, 'bo-', linewidth=2, markersize=8)
        axes[0, 0].set_xlabel('Alpha (Delay Weight)', fontsize=12)
        axes[0, 0].set_ylabel('Average Delay (ms)', fontsize=12)
        axes[0, 0].set_title('Sensitivity to Delay Weight (Alpha)', fontsize=14, fontweight='bold')
        axes[0, 0].grid(alpha=0.3)
        
        # Alpha vs Load Imbalance
        axes[0, 1].plot(alphas, imbalances, 'ro-', linewidth=2, markersize=8)
        axes[0, 1].set_xlabel('Alpha (Delay Weight)', fontsize=12)
        axes[0, 1].set_ylabel('Load Imbalance', fontsize=12)
        axes[0, 1].set_title('Sensitivity to Load Balance Weight (Beta)', fontsize=14, fontweight='bold')
        axes[0, 1].grid(alpha=0.3)
        
        # Super Peers vs Delay
        axes[1, 0].plot(num_sp, sp_delays, 'go-', linewidth=2, markersize=8)
        axes[1, 0].set_xlabel('Number of Super Peers', fontsize=12)
        axes[1, 0].set_ylabel('Average Delay (ms)', fontsize=12)
        axes[1, 0].set_title('Impact of Super Peer Count on Delay', fontsize=14, fontweight='bold')
        axes[1, 0].grid(alpha=0.3)
        
        # Super Peers vs P2P Hit Rate
        axes[1, 1].plot(num_sp, sp_p2p_hits, 'mo-', linewidth=2, markersize=8)
        axes[1, 1].set_xlabel('Number of Super Peers', fontsize=12)
        axes[1, 1].set_ylabel('P2P Hit Rate (%)', fontsize=12)
        axes[1, 1].set_title('Impact of Super Peer Count on P2P Hit Rate', fontsize=14, fontweight='bold')
        axes[1, 1].grid(alpha=0.3)
        
        plt.tight_layout()
        plt.savefig(f'{self.output_dir}/parameter_sensitivity.png', dpi=300, bbox_inches='tight')
        plt.close()
        print("Saved: parameter_sensitivity.png")
    
    def plot_scalability(self):
        """Plot scalability analysis"""
        scalability = self.results['simulations']['scalability']
        
        network_sizes = [s['num_peers'] for s in scalability['network_sizes']]
        elapsed_times = [s['elapsed_time_seconds'] for s in scalability['network_sizes']]
        avg_delays = [s['metrics']['avg_delay_ms'] for s in scalability['network_sizes']]
        p2p_hits = [s['metrics']['p2p_hit_rate'] * 100 for s in scalability['network_sizes']]
        
        fig, axes = plt.subplots(1, 3, figsize=(18, 5))
        
        # Computation time
        axes[0].plot(network_sizes, elapsed_times, 'bo-', linewidth=2, markersize=8)
        axes[0].set_xlabel('Number of Peers', fontsize=12)
        axes[0].set_ylabel('Computation Time (seconds)', fontsize=12)
        axes[0].set_title('Scalability: Computation Time', fontsize=14, fontweight='bold')
        axes[0].grid(alpha=0.3)
        
        # Average delay
        axes[1].plot(network_sizes, avg_delays, 'go-', linewidth=2, markersize=8)
        axes[1].set_xlabel('Number of Peers', fontsize=12)
        axes[1].set_ylabel('Average Delay (ms)', fontsize=12)
        axes[1].set_title('Scalability: Average Delay', fontsize=14, fontweight='bold')
        axes[1].grid(alpha=0.3)
        
        # P2P hit rate
        axes[2].plot(network_sizes, p2p_hits, 'mo-', linewidth=2, markersize=8)
        axes[2].set_xlabel('Number of Peers', fontsize=12)
        axes[2].set_ylabel('P2P Hit Rate (%)', fontsize=12)
        axes[2].set_title('Scalability: P2P Hit Rate', fontsize=14, fontweight='bold')
        axes[2].grid(alpha=0.3)
        
        plt.tight_layout()
        plt.savefig(f'{self.output_dir}/scalability.png', dpi=300, bbox_inches='tight')
        plt.close()
        print("Saved: scalability.png")
    
    def plot_edge_load_distribution(self):
        """Plot edge load distribution for optimized solution"""
        baseline = self.results['simulations']['baseline_comparison']
        optimized = baseline['optimized']
        
        fig, axes = plt.subplots(1, 2, figsize=(14, 5))
        
        # Edge loads comparison
        edges = ['Edge A', 'Edge B']
        loads = [optimized['edge_a_load_kbps'], optimized['edge_b_load_kbps']]
        utilizations = [optimized['edge_a_utilization'] * 100, optimized['edge_b_utilization'] * 100]
        
        axes[0].bar(edges, loads, color=['#4ecdc4', '#45b7d1'])
        axes[0].set_ylabel('Load (kbps)', fontsize=12)
        axes[0].set_title('Edge Server Load Distribution', fontsize=14, fontweight='bold')
        axes[0].grid(axis='y', alpha=0.3)
        
        axes[1].bar(edges, utilizations, color=['#4ecdc4', '#45b7d1'])
        axes[1].set_ylabel('Utilization (%)', fontsize=12)
        axes[1].set_title('Edge Server Utilization', fontsize=14, fontweight='bold')
        axes[1].grid(axis='y', alpha=0.3)
        
        plt.tight_layout()
        plt.savefig(f'{self.output_dir}/edge_load_distribution.png', dpi=300, bbox_inches='tight')
        plt.close()
        print("Saved: edge_load_distribution.png")
    
    def generate_all_plots(self):
        """Generate all visualization plots"""
        print("\n" + "=" * 80)
        print("GENERATING VISUALIZATIONS")
        print("=" * 80)
        
        self.plot_baseline_comparison()
        self.plot_convergence()
        self.plot_parameter_sensitivity()
        self.plot_scalability()
        self.plot_edge_load_distribution()
        
        print("\n" + "=" * 80)
        print("All visualizations generated!")
        print(f"Figures saved to: {self.output_dir}/")
        print("=" * 80)


if __name__ == "__main__":
    visualizer = MetricsVisualizer()
    visualizer.generate_all_plots()


# gnuplot -e "name='CPU during GET'; filename='merge/get-cpu.csv'; outfile='get-cpu.png'" test/performance/cpu.gnuplot
# gnuplot -e "name='CPU during POST'; filename='merge/create-cpu.csv'; outfile='create-cpu.png'" test/performance/cpu.gnuplot
set datafile separator ','

set title name

set xdata time
set timefmt "%Y-%m-%d"

set key autotitle columnhead
set key at screen 1, graph 1
set rmargin 30

set ylabel "CPU"

set terminal png enhanced size 1280,960
set output outfile
set multiplot layout 3,1

plot for [i=2:5] filename using 1:i with lines

unset title
plot for [i=6:9] filename using 1:i with lines
plot for [i=10:13] filename using 1:i with lines

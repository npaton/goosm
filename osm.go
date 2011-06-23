// 48.66,1.90,49.040,2.85
// 48.8077,2.2467,48.9059,2.4245
package main

import (
	"xml"
	"os"
	"log"
	// "time"
	"flag"
	"math"
	"strings"
	"strconv"
	"runtime"
	"archive/zip"
	"path/filepath"
	"compress/bzip2"
)

var (
	filenameptr = flag.String("f", "", "You must specify an osm file - Required")
	boundsptr = flag.String("b", "", "Bounds to limit OSM import. Format minlat,minlon,maxlat,maxlon")
)



func main() {
	flag.Parse()
	filename := *filenameptr
	var bounds Bounds
	if *boundsptr != "" {
		comp := strings.Split(*boundsptr, ",",-1)
		minlat, _ := strconv.Atof64(comp[0])
		minlon, _ := strconv.Atof64(comp[1])
		maxlat, _ := strconv.Atof64(comp[2])
		maxlon, _ := strconv.Atof64(comp[3])
		bounds = Bounds{minlat,minlon,maxlat,maxlon}
		log.Println("loadingWithBounds", bounds)
	} else {
		bounds = Bounds{-90,-180,90,180}
	}
	
	if filename == "" {
		flag.Usage()
		os.Exit(1)
	}
	
	result := OSMFile{Ways:make([]Way,0)}
	
	parser := getParser(filename)
	
	// log.Println("parser", parser)
	
	token, err := parser.Token()
	var currentWay *Way
	shouldConserveCurrentWay := false
	skippedNodeCount := 0
	nodeCount := 0
	wayCount := 0
	usedNodesCount := 0
	nodes := make(map[int]Node)
	for err == nil {
		
		switch token.(type) {
		case nil:
			log.Println("nil Token ?")
			break
		case xml.StartElement:
			startElement, ok := token.(xml.StartElement)
			if !ok { break }
			
			if startElement.Name.Local == "way" {
				currentWay = new(Way)
				for _, attr := range startElement.Attr {
					if attr.Name.Local == "id" {
						v, _ := strconv.Atoi(attr.Value)
						currentWay.Id = v
					}
				}
				
			} else if startElement.Name.Local == "nd" {
				if currentWay == nil { break }
				for _, attr := range startElement.Attr {
					if attr.Name.Local == "ref" {
						v, _ := strconv.Atoi(attr.Value)
						if foundNode, ok := nodes[v]; ok {
							currentWay.Nodes = append(currentWay.Nodes, foundNode)
							usedNodesCount = usedNodesCount + 1
						}
					}
				}
			} else if startElement.Name.Local == "tag" {
				if currentWay == nil { break }
				var key, value string
				for _, attr := range startElement.Attr {
					if attr.Name.Local == "k" {
						key = attr.Value
					} else if attr.Name.Local == "v" {
						value = attr.Value
					}
				}
				if key != "" && value != "" {
					switch key {
					case "highway":
						switch value {
						case "motorway", "motorway_link", "trunk", "trunk_link", "primary", "primary_link", "secondary", "secondary_link", "tertiary":
							shouldConserveCurrentWay = true	
						}
					}
				}
				
			} else if startElement.Name.Local == "node" {
				node := new(Node)
				for _, attr := range startElement.Attr {
					if attr.Name.Local == "id" {
						v, _ := strconv.Atoi(attr.Value)
						node.Id = v
					} else if attr.Name.Local == "lat" {
						v, _ := strconv.Atof64(attr.Value)
						node.Lat = v
					} else if attr.Name.Local == "lon" {
						v, _ := strconv.Atof64(attr.Value)
						node.Lon = v
					}
				}
				
				// log.Println("Node", node, node.Id, node.Lat, node.Lon)
				
				if node.Within(bounds) {
					nodes[node.Id] = *node
					
					nodeCount = nodeCount + 1
					if nodeCount % 100000 == 0 {
						log.Println("Node count:", nodeCount)
					}
				} else {
					skippedNodeCount = skippedNodeCount + 1
					if skippedNodeCount % 100000 == 0 {
						log.Println("Skipped node count:", skippedNodeCount)
					}
				}
				
			}
			break
		case xml.EndElement:
			endElement, ok := token.(xml.EndElement)
			if !ok { break }
			
			if endElement.Name.Local == "way" {
				if shouldConserveCurrentWay && len(currentWay.Nodes) > 5 {
					wayCount = wayCount + 1
					if wayCount % 100000 == 0 {
						log.Println("Way count:", wayCount)
					}
					result.Ways = append(result.Ways, *currentWay)
					log.Println("new accepted way, node count:", len(currentWay.Nodes))
				}
				currentWay = nil
				shouldConserveCurrentWay = false
			}
			break
		case xml.CharData:
		case xml.Comment:
		case xml.ProcInst:
		case xml.Directive:
			break
		}
		
		token, err = parser.Token()
	}
	
	
	log.Println("Node count:", nodeCount)
	log.Println("Skipped node count:", skippedNodeCount)
	log.Println("Way count:", wayCount)
	reallyUsedNodesCount := 0
	for _, way := range result.Ways {
		reallyUsedNodesCount = reallyUsedNodesCount + len(way.Nodes)
	}
	log.Println("Used nodes in ways count", usedNodesCount)
	log.Println("Used nodes in ways count", reallyUsedNodesCount)
	log.Printf("Before GC - bytes = %d - footprint = %d", runtime.MemStats.HeapAlloc, runtime.MemStats.Sys)
	nodes = make(map[int]Node)
	log.Println("Running GC")
	runtime.GC()
	log.Printf("After GC - bytes = %d - footprint = %d", runtime.MemStats.HeapAlloc, runtime.MemStats.Sys)
	
	
	if err != nil && err != os.EOF {
		panic(err)
	}
	
}

func getParser(filename string) *xml.Parser {
	var parser *xml.Parser
	
	if filepath.Ext(filename) == ".zip" {
		zipcontainer, err := zip.OpenReader(filename)
		if err != nil {
			panic(err)
		}
		
		zippedfile := zipcontainer.File[0]
		reader, err := zippedfile.Open()
		if err != nil {
			panic(err)
		}
		
		if filepath.Ext(zippedfile.FileHeader.Name) == ".bz2" {
			log.Println("Uncompressing and unmarshaling XML of zip file")
			parser = xml.NewParser(bzip2.NewReader(reader))
		} else {
			log.Println("Unmarshaling XML of zip file")
			parser = xml.NewParser(reader)
		}
		
		
		reader.Close()
		if err != nil {
			panic(err)
		}
	} else {
		openfile, err := os.Open(filename)
		if err != nil {
			panic(err)
		}
		if filepath.Ext(filename) == ".bz2" {
			log.Println("Uncompressing and unmarshaling XML")
			parser = xml.NewParser(bzip2.NewReader(openfile))
		} else {
			log.Println("Unmarshaling XML")
			parser = xml.NewParser(openfile)
		}
		
		if err != nil {
			panic(err)
		}
	}
	return parser
}

type OSMFile struct {
	Bounds Bounds
	Ways []Way
}

type Bounds struct {
	Minlat float64
	Minlon float64
	Maxlat float64
	Maxlon float64
}


type Node struct {
	Id int
	Lat float64
	Lon float64
}

func (n *Node)Within(bounds Bounds) bool {
	if n.Lat >= bounds.Minlat && n.Lon >= bounds.Minlon && n.Lat <= bounds.Maxlat && n.Lon <= bounds.Maxlon {
		return true
	}
	return false
}

type Way struct {
	Id int
	Nodes []Node
}

type Point struct {
	X, Y float64
}


func (p Point)DistanceTo(other Point) float64 {
	return math.Sqrt(math.Pow(math.Fabs(p.X - other.X), 2) + math.Pow(math.Fabs(p.Y - other.Y), 2))
}

func TriangleAltitude(A, B, C Point) float64 {
	a := A.DistanceTo(B)
	b := A.DistanceTo(C)
	return a*b/2*TriangleCircumradius(a, b, B.DistanceTo(C))
}

// TriangleCircumradius return the circumradius for the given triangle with edge lengths a, b and c
func TriangleCircumradius(a, b, c float64) float64 {
	return (a*b*c)/math.Sqrt((a+b+c)*(b+c-a)*(c+a-b)*(a+b-c))
}


